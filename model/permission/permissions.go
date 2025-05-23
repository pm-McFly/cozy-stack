// Package permission is used to store the permissions for each webapp,
// konnector, sharing, etc.
package permission

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/metadata"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/labstack/echo/v4"
)

// DocTypeVersion represents the doctype version. Each time this document
// structure is modified, update this value
const DocTypeVersion = "1"

// Permission is a storable object containing a set of rules and
// several codes
type Permission struct {
	PID         string            `json:"_id,omitempty"`
	PRev        string            `json:"_rev,omitempty"`
	Type        string            `json:"type,omitempty"`
	SourceID    string            `json:"source_id,omitempty"`
	Permissions Set               `json:"permissions,omitempty"`
	ExpiresAt   interface{}       `json:"expires_at,omitempty"`
	Codes       map[string]string `json:"codes,omitempty"`
	ShortCodes  map[string]string `json:"shortcodes,omitempty"`
	Password    interface{}       `json:"password,omitempty"`

	Client   interface{}            `json:"-"` // Contains the *oauth.Client client pointer for Oauth permission type
	Metadata *metadata.CozyMetadata `json:"cozyMetadata,omitempty"`
}

const (
	// TypeRegister is the value of Permission.Type for the temporary permissions
	// allowed by registerToken
	TypeRegister = "register"

	// TypeWebapp is the value of Permission.Type for an application
	TypeWebapp = "app"

	// TypeKonnector is the value of Permission.Type for an application
	TypeKonnector = "konnector"

	// TypeOauth is the value of Permission.Type for a oauth permission doc
	TypeOauth = "oauth"

	// TypeCLI is the value of Permission.Type for a command-line permission doc
	TypeCLI = "cli"

	// TypeShareByLink is the value of Permission.Type for a share (by link) permission doc
	TypeShareByLink = "share"

	// TypeSharePreview is the value of Permission.Type to preview a
	// cozy-to-cozy sharing
	TypeSharePreview = "share-preview"

	// TypeShareInteract is the value of Permission.Type for reading and
	// writing a note in a shared folder.
	TypeShareInteract = "share-interact"
)

// ID implements jsonapi.Doc
func (p *Permission) ID() string { return p.PID }

// Rev implements jsonapi.Doc
func (p *Permission) Rev() string { return p.PRev }

// DocType implements jsonapi.Doc
func (p *Permission) DocType() string { return consts.Permissions }

// Clone implements couchdb.Doc
func (p *Permission) Clone() couchdb.Doc {
	cloned := *p
	cloned.Codes = make(map[string]string)
	cloned.ShortCodes = make(map[string]string)
	if p.Metadata != nil {
		cloned.Metadata = p.Metadata.Clone()
	}
	for k, v := range p.Codes {
		cloned.Codes[k] = v
	}
	for k, v := range p.ShortCodes {
		cloned.ShortCodes[k] = v
	}
	cloned.Permissions = make([]Rule, len(p.Permissions))
	for i, r := range p.Permissions {
		vals := r.Values
		r.Values = make([]string, len(r.Values))
		copy(r.Values, vals)
		cloned.Permissions[i] = r
	}
	return &cloned
}

// SetID implements jsonapi.Doc
func (p *Permission) SetID(id string) { p.PID = id }

// SetRev implements jsonapi.Doc
func (p *Permission) SetRev(rev string) { p.PRev = rev }

// Expired returns true if the permissions are no longer valid
func (p *Permission) Expired() bool {
	if p.ExpiresAt == nil {
		return false
	}
	if expiresAt, _ := p.ExpiresAt.(string); expiresAt != "" {
		if at, err := time.Parse(time.RFC3339, expiresAt); err == nil {
			return at.Before(time.Now())
		}
	}
	return true
}

// AddRules add some rules to the permission doc
func (p *Permission) AddRules(rules ...Rule) {
	newperms := append(p.Permissions, rules...)
	p.Permissions = newperms
}

// RemoveRule remove a rule from the permission doc
func (p *Permission) RemoveRule(rule Rule) {
	newperms := p.Permissions[:0]
	for _, r := range p.Permissions {
		if r.Title != rule.Title {
			newperms = append(newperms, r)
		}
	}
	p.Permissions = newperms
}

// PatchCodes replace the permission docs codes
func (p *Permission) PatchCodes(codes map[string]string) {
	p.Codes = codes

	// Removing associated shortcodes
	if p.ShortCodes != nil {
		updatedShortcodes := map[string]string{}

		for codeName := range codes {
			for shortcodeName, v := range p.ShortCodes {
				if shortcodeName == codeName {
					updatedShortcodes[shortcodeName] = v
				}
			}
		}
		p.ShortCodes = updatedShortcodes
	}
}

// Revoke destroy a Permission
func (p *Permission) Revoke(db prefixer.Prefixer) error {
	return couchdb.DeleteDoc(db, p)
}

// CanUpdateShareByLink check if the child permissions can be updated by p
// (p can be the parent or it has a superset of the permissions).
func (p *Permission) CanUpdateShareByLink(child *Permission) bool {
	if child.Type != TypeShareByLink {
		return false
	}
	if p.Type != TypeWebapp && p.Type != TypeOauth {
		return false
	}
	return child.SourceID == p.SourceID || child.Permissions.IsSubSetOf(p.Permissions)
}

// GetByID fetch a permission by its ID
func GetByID(db prefixer.Prefixer, id string) (*Permission, error) {
	perm := &Permission{}
	if err := couchdb.GetDoc(db, consts.Permissions, id, perm); err != nil {
		return nil, err
	}
	if perm.Expired() {
		return nil, ErrExpiredToken
	}
	return perm, nil
}

// GetForRegisterToken create a non-persisted permissions doc with hard coded
// registerToken permissions set
func GetForRegisterToken() *Permission {
	return &Permission{
		Type: TypeRegister,
		Permissions: Set{
			Rule{
				Verbs:  Verbs(GET),
				Type:   consts.Settings,
				Values: []string{consts.InstanceSettingsID},
			},
		},
	}
}

// GetForCLI create a non-persisted permissions doc for the command-line
func GetForCLI(claims *Claims) (*Permission, error) {
	set, err := UnmarshalScopeString(claims.Scope)
	if err != nil {
		return nil, err
	}
	pdoc := &Permission{
		Type:        TypeCLI,
		Permissions: set,
	}
	return pdoc, nil
}

// GetForWebapp retrieves the Permission doc for a given webapp
func GetForWebapp(db prefixer.Prefixer, slug string) (*Permission, error) {
	return getFromSource(db, TypeWebapp, consts.Apps, slug)
}

// GetForKonnector retrieves the Permission doc for a given konnector
func GetForKonnector(db prefixer.Prefixer, slug string) (*Permission, error) {
	return getFromSource(db, TypeKonnector, consts.Konnectors, slug)
}

// GetForSharePreview retrieves the Permission doc for a given sharing preview
func GetForSharePreview(db prefixer.Prefixer, sharingID string) (*Permission, error) {
	return getFromSource(db, TypeSharePreview, consts.Sharings, sharingID)
}

// GetForShareInteract retrieves the Permission doc for a given sharing to
// read/write a note
func GetForShareInteract(db prefixer.Prefixer, sharingID string) (*Permission, error) {
	return getFromSource(db, TypeShareInteract, consts.Sharings, sharingID)
}

func getFromSource(db prefixer.Prefixer, permType, docType, slug string) (*Permission, error) {
	var res []Permission
	req := couchdb.FindRequest{
		UseIndex: "by-source-and-type",
		Selector: mango.And(
			mango.Equal("type", permType),
			mango.Equal("source_id", docType+"/"+slug),
		),
		Limit: 1,
	}
	err := couchdb.FindDocs(db, consts.Permissions, &req, &res)
	if err != nil {
		// With a cluster of couchdb, we can have a race condition where we
		// query an index before it has been updated for an app that has
		// just been created.
		// Cf https://issues.apache.org/jira/browse/COUCHDB-3336
		time.Sleep(1 * time.Second)
		err = couchdb.FindDocs(db, consts.Permissions, &req, &res)
		if err != nil {
			return nil, err
		}
	}
	if len(res) == 0 {
		return nil, &couchdb.Error{
			StatusCode: http.StatusNotFound,
			Name:       "not_found",
			Reason:     fmt.Sprintf("no permission doc for %v", slug),
		}
	}
	perm := &res[0]
	if perm.Expired() {
		return nil, ErrExpiredToken
	}
	return perm, nil
}

// GetForShareCode retrieves the Permission doc for a given sharing code
func GetForShareCode(db prefixer.Prefixer, tokenCode string) (*Permission, error) {
	var res couchdb.ViewResponse
	err := couchdb.ExecView(db, couchdb.PermissionsShareByCView, &couchdb.ViewRequest{
		Key:         tokenCode,
		IncludeDocs: true,
	}, &res)
	if err != nil {
		return nil, err
	}

	if len(res.Rows) == 0 {
		msg := fmt.Sprintf("no permission doc for token %v", tokenCode)
		return nil, echo.NewHTTPError(http.StatusForbidden, msg)
	}

	if len(res.Rows) > 1 {
		msg := fmt.Sprintf("Bad state: several permission docs for token %v", tokenCode)
		return nil, echo.NewHTTPError(http.StatusBadRequest, msg)
	}

	perm := &Permission{}
	err = json.Unmarshal(res.Rows[0].Doc, perm)
	if err != nil {
		return nil, err
	}

	if perm.Expired() {
		return nil, ErrExpiredToken
	}

	// Check for sharing made by a webapp/konnector that the app is still
	// present (but not for OAuth). It is not checked in development release,
	// since the --appdir does not create the expected document.
	if !build.IsDevRelease() {
		parts := strings.SplitN(perm.SourceID, "/", 2)
		if len(parts) == 2 {
			var doc couchdb.JSONDoc
			docID := parts[0] + "/" + parts[1]
			if parts[0] == consts.Sharings {
				docID = parts[1]
			}
			if err := couchdb.GetDoc(db, parts[0], docID, &doc); err != nil {
				return nil, ErrExpiredToken
			}
		}
	}
	return perm, nil
}

// GetTokenFromShortcode retrieves the token doc for a given sharing shortcode
func GetTokenFromShortcode(db prefixer.Prefixer, shortcode string) (string, error) {
	token, _, err := GetTokenAndPermissionsFromShortcode(db, shortcode)
	return token, err
}

// GetTokenAndPermissionsFromShortcode retrieves the token and permissions doc for a given sharing shortcode
func GetTokenAndPermissionsFromShortcode(db prefixer.Prefixer, shortcode string) (string, *Permission, error) {
	var res couchdb.ViewResponse

	err := couchdb.ExecView(db, couchdb.PermissionsShareByShortcodeView, &couchdb.ViewRequest{
		Key:         shortcode,
		IncludeDocs: true,
	}, &res)
	if err != nil {
		return "", nil, err
	}

	if len(res.Rows) == 0 {
		msg := fmt.Sprintf("no permission doc for shortcode %v", shortcode)
		return "", nil, echo.NewHTTPError(http.StatusForbidden, msg)
	}

	if len(res.Rows) > 1 {
		msg := fmt.Sprintf("Bad state: several permission docs for shortcode %v", shortcode)
		return "", nil, echo.NewHTTPError(http.StatusBadRequest, msg)
	}

	perm := Permission{}
	err = json.Unmarshal(res.Rows[0].Doc, &perm)

	if err != nil {
		return "", nil, err
	}

	for mail, code := range perm.Codes {
		if mail == res.Rows[0].Value {
			return code, &perm, nil
		}
	}

	return "", nil, fmt.Errorf("Cannot find token for shortcode %s", res.Rows[0].Key)
}

// CreateWebappSet creates a Permission doc for an app
func CreateWebappSet(db prefixer.Prefixer, slug string, set Set, version string) (*Permission, error) {
	existing, _ := GetForWebapp(db, slug)
	if existing != nil {
		return nil, fmt.Errorf("There is already a permission doc for %v", slug)
	}
	// Add metadata
	md, err := metadata.NewWithApp(slug, version, DocTypeVersion)
	if err != nil {
		return nil, err
	}
	return createAppSet(db, TypeWebapp, consts.Apps, slug, set, md)
}

// CreateKonnectorSet creates a Permission doc for a konnector
func CreateKonnectorSet(db prefixer.Prefixer, slug string, set Set, version string) (*Permission, error) {
	existing, _ := GetForKonnector(db, slug)
	if existing != nil {
		return nil, fmt.Errorf("There is already a permission doc for %v", slug)
	}
	// Add metadata
	md, err := metadata.NewWithApp(slug, version, DocTypeVersion)
	if err != nil {
		return nil, err
	}
	return createAppSet(db, TypeKonnector, consts.Konnectors, slug, set, md)
}

func createAppSet(db prefixer.Prefixer, typ, docType, slug string, set Set, md *metadata.CozyMetadata) (*Permission, error) {
	doc := &Permission{
		Type:        typ,
		SourceID:    docType + "/" + slug,
		Permissions: set,
		Metadata:    md,
	}
	err := couchdb.CreateDoc(db, doc)
	if err != nil {
		return nil, err
	}
	return doc, nil
}

// MergeExtraPermissions merges rules from "extraPermissions" set by adding them
// in the "perms" one
func MergeExtraPermissions(perms, extraPermissions Set) (Set, error) {
	var permissions Set

	// Appending the extraPermissions which are not in the target permissions
	for _, ep := range extraPermissions {
		found := false
		for _, p := range perms {
			if ep.Title == p.Title {
				found = true
				break
			}
		}
		if !found {
			permissions = append(permissions, ep)
		}
	}

	// Merging the rules already existing
	for _, rule := range perms {
		found := false
		for _, newRule := range extraPermissions {
			if rule.Title == newRule.Title {
				mergedRule, err := rule.Merge(newRule)
				if err != nil {
					continue
				}
				permissions = append(permissions, *mergedRule)
				found = true
				break
			}
		}
		if !found {
			permissions = append(permissions, rule)
		}
	}

	return permissions, nil
}

// UpdateWebappSet creates a Permission doc for an app
func UpdateWebappSet(db prefixer.Prefixer, slug string, set Set) (*Permission, error) {
	doc, err := GetForWebapp(db, slug)
	if err != nil {
		return nil, err
	}
	return updateAppSet(db, doc, TypeWebapp, consts.Apps, slug, set)
}

// UpdateKonnectorSet creates a Permission doc for a konnector
func UpdateKonnectorSet(db prefixer.Prefixer, slug string, set Set) (*Permission, error) {
	doc, err := GetForKonnector(db, slug)
	if err != nil {
		return nil, err
	}
	return updateAppSet(db, doc, TypeKonnector, consts.Konnectors, slug, set)
}

func updateAppSet(db prefixer.Prefixer, doc *Permission, typ, docType, slug string, set Set) (*Permission, error) {
	doc.Permissions = set
	if doc.Metadata == nil {
		doc.Metadata, _ = metadata.NewWithApp(slug, "", DocTypeVersion)
	} else {
		doc.Metadata.ChangeUpdatedAt()
	}
	err := couchdb.UpdateDoc(db, doc)
	if err != nil {
		return nil, err
	}
	return doc, nil
}

func checkSetPermissions(set Set, parent *Permission) error {
	if parent.Type != TypeWebapp && parent.Type != TypeKonnector && parent.Type != TypeOauth && parent.Type != TypeCLI {
		return ErrOnlyAppCanCreateSubSet
	}
	if !set.IsSubSetOf(parent.Permissions) {
		return ErrNotSubset
	}
	for _, rule := range set {
		// XXX io.cozy.files is allowed and handled with specific code for sharings
		if MatchType(rule, consts.Files) {
			continue
		}
		if err := CheckWritable(rule.Type); err != nil {
			return err
		}
	}
	return nil
}

// CreateShareSet creates a Permission doc for sharing by link
func CreateShareSet(
	db prefixer.Prefixer,
	parent *Permission,
	sourceID string,
	codes, shortcodes map[string]string,
	subdoc Permission,
	expiresAt interface{},
) (*Permission, error) {
	set := subdoc.Permissions
	if err := checkSetPermissions(set, parent); err != nil {
		return nil, err
	}
	// SourceID stays the same, allow quick destruction of all children permissions
	doc := &Permission{
		Type:        TypeShareByLink,
		SourceID:    sourceID,
		Permissions: set,
		Codes:       codes,
		ShortCodes:  shortcodes,
		ExpiresAt:   expiresAt,
		Metadata:    subdoc.Metadata,
	}

	if pass, ok := subdoc.Password.(string); ok && len(pass) > 0 {
		hash, err := crypto.GenerateFromPassphrase([]byte(pass))
		if err != nil {
			return nil, err
		}
		doc.Password = hash
	}

	err := couchdb.CreateDoc(db, doc)
	if err != nil {
		return nil, err
	}

	return doc, nil
}

// CreateSharePreviewSet creates a Permission doc for previewing a sharing
func CreateSharePreviewSet(db prefixer.Prefixer, sharingID string, codes, shortcodes map[string]string, subdoc Permission) (*Permission, error) {
	doc := &Permission{
		Type:        TypeSharePreview,
		Permissions: subdoc.Permissions,
		Codes:       codes,
		ShortCodes:  shortcodes,
		SourceID:    consts.Sharings + "/" + sharingID,
		Metadata:    subdoc.Metadata,
	}
	err := couchdb.CreateDoc(db, doc)
	if err != nil {
		return nil, err
	}
	return doc, nil
}

// CreateShareInteractSet creates a Permission doc for reading/writing a note
// inside a sharing
func CreateShareInteractSet(db prefixer.Prefixer, sharingID string, codes map[string]string, subdoc Permission) (*Permission, error) {
	doc := &Permission{
		Type:        TypeShareInteract,
		Permissions: subdoc.Permissions,
		Codes:       codes,
		SourceID:    consts.Sharings + "/" + sharingID,
		Metadata:    subdoc.Metadata,
	}
	err := couchdb.CreateDoc(db, doc)
	if err != nil {
		return nil, err
	}
	return doc, nil
}

// ForceWebapp creates or updates a Permission doc for a given webapp
func ForceWebapp(db prefixer.Prefixer, slug string, set Set) error {
	existing, _ := GetForWebapp(db, slug)
	doc := &Permission{
		Type:        TypeWebapp,
		SourceID:    consts.Apps + "/" + slug,
		Permissions: set,
	}
	if existing == nil {
		return couchdb.CreateDoc(db, doc)
	}

	doc.SetID(existing.ID())
	doc.SetRev(existing.Rev())
	return couchdb.UpdateDoc(db, doc)
}

// ForceKonnector creates or updates a Permission doc for a given konnector
func ForceKonnector(db prefixer.Prefixer, slug string, set Set) error {
	existing, _ := GetForKonnector(db, slug)
	doc := &Permission{
		Type:        TypeKonnector,
		SourceID:    consts.Konnectors + "/" + slug,
		Permissions: set,
	}
	if existing == nil {
		return couchdb.CreateDoc(db, doc)
	}

	doc.SetID(existing.ID())
	doc.SetRev(existing.Rev())
	return couchdb.UpdateDoc(db, doc)
}

// DestroyWebapp remove all Permission docs for a given app
func DestroyWebapp(db prefixer.Prefixer, slug string) error {
	return destroyApp(db, TypeWebapp, consts.Apps, slug)
}

// DestroyKonnector remove all Permission docs for a given konnector
func DestroyKonnector(db prefixer.Prefixer, slug string) error {
	return destroyApp(db, TypeKonnector, consts.Konnectors, slug)
}

func destroyApp(db prefixer.Prefixer, permType, docType, slug string) error {
	var res []Permission
	err := couchdb.FindDocs(db, consts.Permissions, &couchdb.FindRequest{
		UseIndex: "by-source-and-type",
		Selector: mango.And(
			mango.Equal("source_id", docType+"/"+slug),
			mango.Equal("type", permType),
		),
		Limit: 1000,
	}, &res)
	if err != nil {
		return err
	}
	for _, p := range res {
		err := couchdb.DeleteDoc(db, &p)
		if err != nil {
			return err
		}
	}
	return nil
}

// GetPermissionsForIDs gets permissions for several IDs
// returns for every id the combined allowed verbset
func GetPermissionsForIDs(db prefixer.Prefixer, doctype string, ids []string) (map[string]*VerbSet, error) {
	var res struct {
		Rows []struct {
			ID    string   `json:"id"`
			Key   []string `json:"key"`
			Value *VerbSet `json:"value"`
		} `json:"rows"`
	}

	keys := make([]interface{}, len(ids))
	for i, id := range ids {
		keys[i] = []string{doctype, "_id", id}
	}

	err := couchdb.ExecView(db, couchdb.PermissionsShareByDocView, &couchdb.ViewRequest{
		Keys: keys,
	}, &res)
	if err != nil {
		return nil, err
	}

	result := make(map[string]*VerbSet)
	for _, row := range res.Rows {
		if _, ok := result[row.Key[2]]; ok {
			result[row.Key[2]].Merge(row.Value)
		} else {
			result[row.Key[2]] = row.Value
		}
	}

	return result, nil
}

// GetPermissionsByDoctype returns the list of all permissions of the given
// type (shared-with-me by example) that have at least one rule for the given
// doctype. The cursor will be modified in place.
func GetPermissionsByDoctype(db prefixer.Prefixer, permType, doctype string, cursor couchdb.Cursor) ([]Permission, error) {
	req := &couchdb.ViewRequest{
		Key:         [2]interface{}{doctype, permType},
		IncludeDocs: true,
	}
	cursor.ApplyTo(req)

	var res couchdb.ViewResponse
	err := couchdb.ExecView(db, couchdb.PermissionsByDoctype, req, &res)
	if err != nil {
		return nil, err
	}
	cursor.UpdateFrom(&res)

	result := make([]Permission, len(res.Rows))

	for i, row := range res.Rows {
		var doc Permission
		err := json.Unmarshal(row.Doc, &doc)
		if err != nil {
			return nil, err
		}
		result[i] = doc
	}

	return result, nil
}
