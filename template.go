package memecreator

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/file"
	"google.golang.org/appengine/log"
	"google.golang.org/appengine/memcache"
)

const (
	// MaxTemplateSize is 5MB
	MaxTemplateSize = 5 * 1024 * 1024

	// TemplateKind is the name of ind in Datastore.
	TemplateKind = "Template"
)

// Template is type used for storing details about template.
type Template struct {
	Created  time.Time `json:"created"`
	Filename string    `json:"filename"`
}

// TemplateResponse is type returned as response from API.
type TemplateResponse struct {
	ID string `json:"id"`
	Template
}

// TemplatesHandler handles actions getting templates or creating new template.
func TemplatesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		GetTemplatesHandler(w, r)
		return
	}

	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprint(w, `{"error":"method not allowed"}`)
		return
	}

	PostTemplatesHandler(w, r)
}

// GetTemplatesHandler handles getting existing template by id.
func GetTemplatesHandler(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)

	var templates []*TemplateResponse
	keys, err := datastore.NewQuery(TemplateKind).
		Order("-Created").
		GetAll(ctx, &templates)
	if err != nil {
		log.Errorf(ctx, "fetching template from datastore failed, error %s", err)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"something went wrong ;("}`)
		return
	}

	for i, t := range templates {
		t.ID = keys[i].Encode()
	}

	if templates == nil {
		templates = make([]*TemplateResponse, 0)
	}

	resp := map[string]interface{}{
		"templates": templates,
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Errorf(ctx, "fetching template from datastore failed, error %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"something went wrong ;("}`)
		return
	}
}

// PostTemplatesHandler handles creating new template.
func PostTemplatesHandler(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)

	if err := r.ParseMultipartForm(MaxTemplateSize); err != nil {
		log.Errorf(ctx, "parsing multipart form failed, error: %s", err)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":"template is too large"}`)
		return
	}

	templateFile, templateHandler, err := r.FormFile("template")
	if err != nil {
		log.Errorf(ctx, "getting multipart form template object failed, error: %s", err)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":"missing template file in request"}`)
		return
	}
	defer templateFile.Close()

	storageClient, err := storage.NewClient(ctx)
	if err != nil {
		log.Errorf(ctx, "getting storage client failed, error: %s", err)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"something went wrong ;("}`)
		return
	}

	bucketName, err := file.DefaultBucketName(ctx)
	if err != nil {
		log.Errorf(ctx, "getting storage bucket name failed, error: %s", err)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"something went wrong ;("}`)
		return
	}

	cso := storageClient.
		Bucket(bucketName).
		Object(templateHandler.Filename)

	csow := cso.NewWriter(ctx)
	if _, err := io.Copy(csow, templateFile); err != nil {
		log.Errorf(ctx, "copying data to cloud storage failed, error: %s", err)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"something went wrong ;("}`)
		return
	}

	if err := csow.Close(); err != nil {
		log.Errorf(ctx, "closing cloud storage writer failed, error: %s", err)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"something went wrong ;("}`)
		return
	}

	if err := cso.ACL().Set(ctx, storage.AllUsers, storage.RoleReader); err != nil {
		log.Errorf(ctx, "setting acl on template cloud storage object failed, error: %s", err)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"something went wrong ;("}`)
		return
	}

	templateKey, err := datastore.Put(
		ctx,
		datastore.NewIncompleteKey(ctx, TemplateKind, nil),
		&Template{
			Created:  time.Now(),
			Filename: templateHandler.Filename,
		},
	)
	if err != nil {
		log.Errorf(ctx, "storing template in datastore failed, error: %s", err)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"something went wrong ;("}`)
		return
	}

	item := &memcache.Item{
		Key:   templateKey.Encode(),
		Value: []byte(templateHandler.Filename),
	}
	if err := memcache.Add(ctx, item); err != nil {
		log.Errorf(ctx, "storing template in memcache failed, error: %s", err)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"something went wrong ;("}`)
		return
	}

	w.Header().Set("Location", fmt.Sprintf("/templates/%s", templateKey.Encode()))
	w.WriteHeader(http.StatusCreated)
}

// TemplateHandler handles getting existing template.
func TemplateHandler(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)

	if r.Method != http.MethodGet {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprint(w, `{"error":"method not allowed"}`)
		return
	}

	u, err := url.Parse(r.RequestURI)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		return
	}

	templateKey, err := datastore.DecodeKey(strings.TrimPrefix(u.Path, "/templates/"))
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		return
	}

	var template Template
	if err := datastore.Get(
		ctx,
		templateKey,
		&template,
	); err == datastore.ErrNoSuchEntity {
		w.WriteHeader(http.StatusNotFound)
		return
	} else if err != nil {
		log.Errorf(ctx, "getting template from datastore failed, error: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"something went wrong ;("}`)
		return
	}

	resp := map[string]interface{}{
		"template": template,
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Errorf(ctx, "fetching template from datastore failed, error %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"something went wrong ;("}`)
		return
	}
}
