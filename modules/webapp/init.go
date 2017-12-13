package webapp

import (
	"net/http"

	"github.com/czertbytes/memecreator"
)

// Init acts as main function for attaching handlers.
func init() {
	http.HandleFunc("/templates", memecreator.TemplatesHandler)
	http.HandleFunc("/templates/", memecreator.TemplateHandler)
	http.HandleFunc("/memes", memecreator.MemesHandler)
	http.HandleFunc("/memes/", memecreator.MemeHandler)
	http.HandleFunc("/worker", memecreator.WorkerHandler)
}
