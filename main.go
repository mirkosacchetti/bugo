package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"time"
)

type Post struct {
	ID       string
	Title    string
	Subtitle string
	Path     string
	Date     time.Time
	IsDraft  bool
	HTMLBody string
}

type Page struct {
	Title string
	Path  string
	Body  string
}

var devMode bool

func main() {
	args := os.Args
	if len(args) > 1 {
		switch args[1] {
		case "dev": // show current post in browser
			devMode = true
			fs := http.FileServer(http.Dir("public/"))
			http.HandleFunc("/", indexHandler)
			http.Handle("/static/", fs)
			http.HandleFunc("/post/{id}", postHandler)
			http.HandleFunc("/dev/sse", sseHandler)
			if err := startWatcher("templates", "public", "posts"); err != nil {
				log.Printf("watcher error: %v", err)
			}
			log.Printf("Server on http://localhost:3000")
			log.Fatal(http.ListenAndServe(":3000", nil))

		case "pub": // compile and pubblish the new post
			os.MkdirAll("public/post", 0755)
			posts, err := loadPosts()
			if err != nil {
				log.Printf("watcher error: %v", err)
			}
			for _, post := range posts {
				pf, err := os.Create("public/post/" + post.ID + ".html")
				if err != nil {
					log.Fatal(err)
				}
				renderPost(pf, post)
				pf.Close()
			}
			pf, err := os.Create("public/index.html")
			if err != nil {
				log.Fatal(err)
			}
			renderIndex(pf, posts)
			pf.Close()

		}
	}
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	posts, err := loadPosts()
	if err != nil {
		fmt.Fprintln(w, err.Error())
	}
	renderIndex(w, posts)
}

func postHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	id = strings.TrimSuffix(id, ".html")
	post, err := loadPost(id)
	if err != nil {
		fmt.Fprintln(w, err.Error())
	}
	renderPost(w, *post)
}

func parseTemplates(files ...string) *template.Template {
	if devMode {
		files = append(files, "templates/hot_reload.html")
	}
	tmpl := template.New("master.html").Funcs(template.FuncMap{
		"formatDate": formatDate,
	})
	return template.Must(tmpl.ParseFiles(files...))
}

func renderPost(w io.Writer, post Post) {
	tmpl := parseTemplates("templates/master.html", "templates/post.html")
	tmpl.ExecuteTemplate(w, "master.html", post)
}
func renderIndex(w io.Writer, posts []Post) {
	tmpl := parseTemplates("templates/master.html", "templates/index.html")
	tmpl.ExecuteTemplate(w, "master.html", posts)
}

func loadPost(fn string) (*Post, error) {
	files, err := os.ReadDir("posts")
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		if filepath.Ext(file.Name()) != ".md" {
			continue
		}
		p := strings.TrimSuffix(file.Name(), ".md")
		if p == fn {
			content, err := os.ReadFile("posts/" + file.Name())
			if err != nil {
				return nil, err
			}
			parts := strings.SplitN(string(content), "+++", 3)
			if len(parts) != 3 {
				return nil, fmt.Errorf("invalid front matter: expected opening and closing +++ delimiters")
			}
			// parts[0] is empty because the file starts with +++.
			post := parseFrontMatter(parts[1])
			post.HTMLBody = parseMD(parts[2])
			//		if post.IsDraft {
			return &post, nil
			//		}
		}
	}
	return nil, nil
}

func loadPosts() ([]Post, error) {
	var posts []Post
	files, err := os.ReadDir("posts")
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		if filepath.Ext(file.Name()) != ".md" {
			continue
		}
		content, err := os.ReadFile("posts/" + file.Name())
		if err != nil {
			return nil, err
		}
		parts := strings.SplitN(string(content), "+++", 3)
		if len(parts) != 3 {
			return posts, fmt.Errorf("invalid front matter: expected opening and closing +++ delimiters")
		}
		// parts[0] is empty because the file starts with +++.
		post := parseFrontMatter(parts[1])
		post.ID = strings.TrimSuffix(file.Name(), ".md")
		post.HTMLBody = parseMD(parts[2])
		if !post.IsDraft {
			posts = append(posts, post)
		}
	}
	sort.Slice(posts, func(i, j int) bool {
		return posts[i].Date.After(posts[j].Date)
	})
	return posts, nil
}

func formatDate(t time.Time) string {
	return t.Format("02 Jan 2006")
}

func parseMD(m string) string {
	var html string
	for line := range strings.Lines(m) {
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")
		if len(line) > 0 {
			html = html + string(parseMarkdown([]byte(line)))
		}
	}
	return html
}
