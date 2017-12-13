package memecreator

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/log"
	"google.golang.org/appengine/taskqueue"
)

const (
	// MemeKind is the name of kind in Datastore.
	MemeKind = "Meme"
)

const (
	// MemePublicURLPrefix is url prefix used for all generated memes.
	MemePublicURLPrefix = "https://storage.googleapis.com/dh-meme-creator.appspot.com"
)

// Meme is type used for storing details about meme.
type Meme struct {
	Created    time.Time `json:"created"`
	Status     string    `json:"status"`
	TemplateID string    `json:"template_id"`
	Top        string    `json:"top"`
	Bottom     string    `json:"bottom"`
}

// CreateMemeCommand is type used when creating new meme.
type CreateMemeCommand struct {
	TemplateID string `json:"template_id"`
	Top        string `json:"top"`
	Bottom     string `json:"bottom"`
}

// MemeResponse is type returned as response from API.
type MemeResponse struct {
	ID string `json:"id"`
	Meme
	PublicURL string `json:"public_url"`
}

// MemesHandler handles actions getting existing memes or creating new meme.
func MemesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		GetMemesHandler(w, r)
		return
	}

	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprint(w, `{"error":"method not allowed"}`)
		return
	}

	PostMemesHandler(w, r)
}

// GetMemesHandler handles getting existing memes by id.
func GetMemesHandler(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)

	if r.Method != http.MethodGet {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprint(w, `{"error":"method not allowed"}`)
		return
	}

	var memes []*MemeResponse
	keys, err := datastore.NewQuery(MemeKind).
		Order("-Created").
		Limit(100).
		GetAll(ctx, &memes)
	if err != nil {
		log.Errorf(ctx, "fetching meme from datastore failed, error %s", err)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"something went wrong ;("}`)
		return
	}

	for i, m := range memes {
		m.ID = keys[i].Encode()
		m.PublicURL = fmt.Sprintf("%s/%s.png", MemePublicURLPrefix, m.ID)
	}

	if memes == nil {
		memes = make([]*MemeResponse, 0)
	}

	resp := map[string]interface{}{
		"memes": memes,
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Errorf(ctx, "fetching template from datastore failed, error %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"something went wrong ;("}`)
		return
	}
}

// PostMemesHandler handles creating new meme.
func PostMemesHandler(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)

	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprint(w, `{"error":"method not allowed"}`)
		return
	}

	contentType := r.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad media type"}`))
		return
	}

	cmd := new(CreateMemeCommand)

	switch mediaType {
	case "application/json":
		dec := json.NewDecoder(r.Body)
		for {
			if err := dec.Decode(&cmd); err == io.EOF {
				break
			} else if err != nil {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"error":"parsing request failed"}`))
				return
			}
		}
	default:
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"unsupported content type"}`))
		return
	}

	memeKey, err := datastore.Put(
		ctx,
		datastore.NewIncompleteKey(ctx, MemeKind, nil),
		&Meme{
			Created:    time.Now(),
			Status:     "created",
			TemplateID: cmd.TemplateID,
			Top:        cmd.Top,
			Bottom:     cmd.Bottom,
		},
	)
	if err != nil {
		log.Errorf(ctx, "storing meme in datastore failed, error: %s", err)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"something went wrong ;("}`)
		return
	}

	createMemeTask := taskqueue.NewPOSTTask("/worker", url.Values{
		"meme_id": []string{memeKey.Encode()},
	})

	if _, err := taskqueue.Add(ctx, createMemeTask, ""); err != nil {
		log.Errorf(ctx, "adding task in queue failed, error: %s", err)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"something went wrong ;("}`)
		return
	}

	w.Header().Set("Location", fmt.Sprintf("/memes/%s", memeKey.Encode()))
	w.WriteHeader(http.StatusCreated)
}

// MemeHandler handles getting existing meme.
func MemeHandler(w http.ResponseWriter, r *http.Request) {
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

	memeID := strings.TrimPrefix(u.Path, "/memes/")
	memeKey, err := datastore.DecodeKey(memeID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		return
	}

	var meme Meme
	if err := datastore.Get(
		ctx,
		memeKey,
		&meme,
	); err == datastore.ErrNoSuchEntity {
		w.WriteHeader(http.StatusNotFound)
		return
	} else if err != nil {
		log.Errorf(ctx, "getting meme from datastore failed, error: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"something went wrong ;("}`)
		return
	}

	mr := MemeResponse{
		ID:        memeID,
		Meme:      meme,
		PublicURL: fmt.Sprintf("%s/%s.png", MemePublicURLPrefix, memeID),
	}

	resp := map[string]interface{}{
		"meme": mr,
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Errorf(ctx, "fetching meme from datastore failed, error %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"something went wrong ;("}`)
		return
	}
}
