package memecreator

import (
	"fmt"
	"image"
	_ "image/jpeg" // allows decoding JPEG files too
	"image/png"
	"net/http"

	"cloud.google.com/go/storage"
	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/file"
	"google.golang.org/appengine/log"
)

// WorkerHandler is queue handler for generating new meme image.
func WorkerHandler(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)

	memeID := r.FormValue("meme_id")

	memeKey, err := datastore.DecodeKey(memeID)
	if err != nil {
		log.Errorf(ctx, "decoding meme key failed, error: %s", err)
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
		log.Errorf(ctx, "meme key %s not found", memeID)
		w.WriteHeader(http.StatusNotFound)
		return
	} else if err != nil {
		log.Errorf(ctx, "getting meme from datastore failed, error: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	templateKey, err := datastore.DecodeKey(meme.TemplateID)
	if err != nil {
		log.Errorf(ctx, "decoding template key failed, error: %s", err)
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
		log.Errorf(ctx, "template key %s not found", memeID)
		w.WriteHeader(http.StatusNotFound)
		return
	} else if err != nil {
		log.Errorf(ctx, "getting template from datastore failed, error: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

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

	templateReader, err := storageClient.
		Bucket(bucketName).
		Object(template.Filename).
		NewReader(ctx)
	if err != nil {
		log.Errorf(ctx, "getting storage object failed, error: %s", err)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"something went wrong ;("}`)
		return
	}
	defer templateReader.Close()

	templateImage, _, err := image.Decode(templateReader)
	if err != nil {
		log.Errorf(ctx, "decoding template image failed, error: %s", err)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"something went wrong ;("}`)
		return
	}

	memeResult, err := RenderMeme(WorkerFontBytes, templateImage, meme.Top, meme.Bottom)
	if err != nil {
		log.Errorf(ctx, "rendering meme failed, error: %s", err)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"something went wrong ;("}`)
		return
	}

	cso := storageClient.
		Bucket(bucketName).
		Object(memeID + ".png")

	csow := cso.NewWriter(ctx)
	if err := png.Encode(csow, memeResult); err != nil {
		log.Errorf(ctx, "copying rendered file to cloud storage failed, error: %s", err)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"something went wrong ;("}`)
		return
	}

	if err := csow.Close(); err != nil {
		log.Errorf(ctx, "closing rendered file storage writer failed, error: %s", err)
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

	meme.Status = "done"
	if _, err := datastore.Put(
		ctx,
		memeKey,
		&meme,
	); err != nil {
		log.Errorf(ctx, "updating meme in datastore failed, error: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}
