package statik

import (
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/assets"
	modelAsset "github.com/cozy/cozy-stack/pkg/assets/model"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/i18n"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

var (
	templatesList = []string{
		"authorize.html",
		"authorize_move.html",
		"authorize_sharing.html",
		"compat.html",
		"confirm_auth.html",
		"confirm_flagship.html",
		"error.html",
		"import.html",
		"install_flagship_app.html",
		"instance_blocked.html",
		"login.html",
		"magic_link_twofactor.html",
		"move_confirm.html",
		"move_delegated_auth.html",
		"move_in_progress.html",
		"move_link.html",
		"move_vault.html",
		"need_onboarding.html",
		"new_app_available.html",
		"oidc_login.html",
		"oidc_twofactor.html",
		"passphrase_choose.html",
		"passphrase_reset.html",
		"share_by_link_password.html",
		"sharing_discovery.html",
		"oauth_clients_limit_exceeded.html",
		"twofactor.html",
	}
)

const (
	assetsPrefix    = "/assets"
	assetsExtPrefix = "/assets/ext"
)

var (
	ErrInvalidPath = errors.New("invalid file path")
)

// AssetRenderer is an interface for both a template renderer and an asset HTTP
// handler.
type AssetRenderer interface {
	echo.Renderer
	http.Handler
}

type dir string

func (d dir) Open(name string) (http.File, error) {
	if filepath.Separator != '/' && strings.ContainsRune(name, filepath.Separator) {
		return nil, fmt.Errorf("%w: invalid character", ErrInvalidPath)
	}
	dir := string(d)
	if dir == "" {
		dir = "."
	}
	name, _ = ExtractAssetID(name)
	fullName := filepath.Join(dir, filepath.FromSlash(path.Clean("/"+name)))
	f, err := os.Open(fullName)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// NewDirRenderer returns a renderer with assets opened from a specified local
// directory.
func NewDirRenderer(assetsPath string) (AssetRenderer, error) {
	list := make([]string, len(templatesList))
	for i, name := range templatesList {
		list[i] = filepath.Join(assetsPath, "templates", name)
	}

	t := template.New("stub")
	h := http.StripPrefix(assetsPrefix, http.FileServer(dir(assetsPath)))
	middlewares.FuncsMap = template.FuncMap{
		"t":         fmt.Sprintf,
		"tHTML":     fmt.Sprintf,
		"split":     strings.Split,
		"replace":   strings.Replace,
		"hasSuffix": strings.HasSuffix,
		"asset":     basicAssetPath,
		"ext":       fileExtension,
		"basename":  basename,
		"filetype":  filetype,
	}

	var err error
	t, err = t.Funcs(middlewares.FuncsMap).ParseFiles(list...)
	if err != nil {
		return nil, fmt.Errorf("Can't load the assets from %q: %s", assetsPath, err)
	}

	return &renderer{t: t, Handler: h}, nil
}

// NewRenderer return a renderer with assets loaded form their packed
// representation into the binary.
func NewRenderer() (AssetRenderer, error) {
	t := template.New("stub")

	middlewares.FuncsMap = template.FuncMap{
		"t":         fmt.Sprintf,
		"tHTML":     fmt.Sprintf,
		"split":     strings.Split,
		"replace":   strings.Replace,
		"hasSuffix": strings.HasSuffix,
		"asset":     AssetPath,
		"ext":       fileExtension,
		"basename":  basename,
		"filetype":  filetype,
	}

	for _, name := range templatesList {
		tmpl := t.New(name).Funcs(middlewares.FuncsMap)
		f, err := assets.Open("/templates/"+name, config.DefaultInstanceContext)
		if err != nil {
			return nil, fmt.Errorf("Can't load asset %q: %s", name, err)
		}
		b, err := io.ReadAll(f)
		if err != nil {
			return nil, err
		}
		if _, err = tmpl.Parse(string(b)); err != nil {
			return nil, err
		}
	}

	return &renderer{t: t, Handler: NewHandler()}, nil
}

type renderer struct {
	http.Handler
	t *template.Template
}

func (r *renderer) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	var funcMap template.FuncMap
	i, ok := middlewares.GetInstanceSafe(c)
	if ok {
		funcMap = template.FuncMap{
			"t":     i.Translate,
			"tHTML": i18n.TranslatorHTML(i.Locale, i.ContextName),
		}
	} else {
		lang := GetLanguageFromHeader(c.Request().Header)
		funcMap = template.FuncMap{
			"t":     i18n.Translator(lang, ""),
			"tHTML": i18n.TranslatorHTML(lang, ""),
		}
	}
	var t *template.Template
	var err error
	if m, ok := data.(echo.Map); ok {
		if context, ok := m["ContextName"].(string); ok {
			if i != nil {
				assets.LoadContextualizedLocale(context, i.Locale)
			}
			if f, err := assets.Open("/templates/"+name, context); err == nil {
				b, err := io.ReadAll(f)
				if err != nil {
					return err
				}
				tmpl := template.New(name).Funcs(middlewares.FuncsMap)
				if _, err = tmpl.Parse(string(b)); err != nil {
					return err
				}
				t = tmpl
			}
		}
	}
	if t == nil {
		t, err = r.t.Clone()
		if err != nil {
			return err
		}
	}

	// Add some CSP for rendered web pages
	if !config.GetConfig().CSPDisabled {
		middlewares.AppendCSPRule(c, "default-src", "'self'")
		middlewares.AppendCSPRule(c, "img-src", "'self' data:")
	}

	return t.Funcs(funcMap).ExecuteTemplate(w, name, data)
}

// AssetPath return the fullpath with unique identifier for a given asset file.
func AssetPath(domain, name string, context ...string) string {
	ctx := config.DefaultInstanceContext
	if len(context) > 0 && context[0] != "" {
		ctx = context[0]
	}
	f, ok := assets.Head(name, ctx)
	if !ok {
		logger.WithNamespace("assets").WithFields(logger.Fields{
			"domain":  domain,
			"name":    name,
			"context": ctx,
		}).Infof("Cannot find asset")
	}

	if ok {
		name = f.NameWithSum
		if !f.IsCustom {
			context = nil
		}
	}
	return assetPath(domain, name, context...)
}

// basicAssetPath is used with DirRenderer to skip the sum in URL, and avoid
// caching the assets.
func basicAssetPath(domain, name string, context ...string) string {
	ctx := config.DefaultInstanceContext
	if len(context) > 0 && context[0] != "" {
		ctx = context[0]
	}
	f, ok := assets.Head(name, ctx)
	if ok && !f.IsCustom {
		context = nil
	}
	return assetPath(domain, name, context...)
}

func assetPath(domain, name string, context ...string) string {
	if len(context) > 0 && context[0] != "" {
		name = path.Join(assetsExtPrefix, url.PathEscape(context[0]), name)
	} else {
		name = path.Join(assetsPrefix, name)
	}
	if domain != "" {
		return "//" + domain + name
	}
	return name
}

// Handler implements http.handler for a subpart of the available assets on a
// specified prefix.
type Handler struct{}

// NewHandler returns a new handler
func NewHandler() Handler {
	return Handler{}
}

// ServeHTTP implements the http.Handler interface.
func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// The URL path should be formed in one on those forms:
	// /assets/:file...
	// /assets/ext/(:context-name)/:file...

	var id, name, context string

	if strings.HasPrefix(r.URL.Path, assetsExtPrefix+"/") {
		nameWithContext := strings.TrimPrefix(r.URL.Path, assetsExtPrefix+"/")
		nameWithContextSplit := strings.SplitN(nameWithContext, "/", 2)
		if len(nameWithContextSplit) != 2 {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}
		context = nameWithContextSplit[0]
		name = nameWithContextSplit[1]
	} else {
		name = strings.TrimPrefix(r.URL.Path, assetsPrefix)
		if inst, err := lifecycle.GetInstance(r.Host); err == nil {
			context = inst.ContextName
		}
	}

	name, id = ExtractAssetID(name)
	if len(name) > 0 && name[0] != '/' {
		name = "/" + name
	}

	f, ok := assets.Get(name, context)
	if !ok {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	checkETag := id == ""
	h.ServeFile(w, r, f, checkETag)
}

// ServeFile can be used to respond with an asset file to an HTTP request
func (h *Handler) ServeFile(w http.ResponseWriter, r *http.Request, f *modelAsset.Asset, checkETag bool) {
	if checkETag && utils.CheckPreconditions(w, r, f.Etag) {
		return
	}

	headers := w.Header()
	headers.Set(echo.HeaderContentType, f.Mime)
	headers.Set(echo.HeaderContentLength, f.Size())
	headers.Set(echo.HeaderVary, echo.HeaderOrigin)
	headers.Add(echo.HeaderVary, echo.HeaderAcceptEncoding)

	acceptsBrotli := strings.Contains(r.Header.Get(echo.HeaderAcceptEncoding), "br")
	if acceptsBrotli {
		headers.Set(echo.HeaderContentEncoding, "br")
		headers.Set(echo.HeaderContentLength, f.BrotliSize())
	} else {
		headers.Set(echo.HeaderContentLength, f.Size())
	}

	if checkETag {
		headers.Set("Etag", f.Etag)
		headers.Set("Cache-Control", "no-cache, public")
	} else {
		headers.Set("Cache-Control", "max-age=31536000, public, immutable")
	}

	if r.Method == http.MethodGet {
		if acceptsBrotli {
			_, _ = io.Copy(w, f.BrotliReader())
		} else {
			_, _ = io.Copy(w, f.Reader())
		}
	}
}

// GetLanguageFromHeader return the language tag given the Accept-Language
// header.
func GetLanguageFromHeader(header http.Header) (lang string) {
	lang = consts.DefaultLocale
	acceptHeader := header.Get("Accept-Language")
	if acceptHeader == "" {
		return
	}
	acceptLanguages := utils.SplitTrimString(acceptHeader, ",")
	for _, tag := range acceptLanguages {
		// tag may contain a ';q=' for a quality factor that we do not take into
		// account.
		if i := strings.Index(tag, ";q="); i >= 0 {
			tag = tag[:i]
		}
		// tag may contain a '-' to introduce a country variante, that we do not
		// take into account.
		if i := strings.IndexByte(tag, '-'); i >= 0 {
			tag = tag[:i]
		}
		if utils.IsInArray(tag, consts.SupportedLocales) {
			lang = tag
			return
		}
	}
	return
}

// ExtractAssetID checks if a long hexadecimal string is contained in given
// file path and returns the original file name and ID (if any). For instance
// <foo.badbeedbadbeef.min.js> = <foo.min.js, badbeefbadbeef>
func ExtractAssetID(file string) (string, string) {
	var id string
	base := path.Base(file)
	off1 := strings.IndexByte(base, '.') + 1
	if off1 < len(base) {
		off2 := off1 + strings.IndexByte(base[off1:], '.')
		if off2 > off1 {
			if s := base[off1:off2]; isLongHexString(s) || s == "immutable" {
				dir := path.Dir(file)
				id = s
				file = base[:off1-1] + base[off2:]
				if dir != "." {
					file = path.Join(dir, file)
				}
			}
		}
	}
	return file, id
}

func isLongHexString(s string) bool {
	if len(s) < 10 {
		return false
	}
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		case c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}

func fileExtension(filename string) string {
	return path.Ext(filename)
}

func basename(filename string) string {
	ext := fileExtension(filename)
	return strings.TrimSuffix(filename, ext)
}

func filetype(mime string) string {
	if mime == consts.NoteMimeType {
		return "note"
	}
	_, class := vfs.ExtractMimeAndClass(mime)
	if class == "shortcut" || class == "application" {
		return "files"
	}
	return class
}
