package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/safebrowsing"

	"github.com/gorilla/mux"

	_ "github.com/mattn/go-sqlite3"
)

// links validated with https://github.com/google/safebrowsing

func main() {
	if err := run(os.Args, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}

// Category is a page/collection of links
type Category struct {
	ID           int64
	Name         string
	URLStub      string
	Passcode     string
	PageTopBlurb sql.NullString
}

// Link is a shared URL with name, notes
type Link struct {
	ID       int64
	Category string
	Name     string
	URL      string
	Notes    sql.NullString
	Added    sql.NullString
	Safe     int
}

type Server struct {
	DB *sql.DB
	SB *safebrowsing.SafeBrowser
}

func run(args []string, stdout io.Writer) error {

	db, _ := sql.Open("sqlite3", "linkit.db")
	defer db.Close()

	sb, err := safebrowsing.NewSafeBrowser(safebrowsing.Config{
		APIKey:    GoogleAPIKey(db),
		Logger:    os.Stderr,
		ServerURL: safebrowsing.DefaultServerURL,
	})
	if err != nil {
		log.Fatalf("Unable to initialize Safe Browsing client: %v", err)
	}

	server := Server{
		DB: db,
		SB: sb,
	}

	//
	// routes
	//
	r := mux.NewRouter()
	static := http.FileServer(http.Dir("assets"))
	r.PathPrefix("/css/").Handler(static)
	r.PathPrefix("/img/").Handler(static)
	r.PathPrefix("/js/").Handler(static)
	r.HandleFunc("/favicon.ico", http.NotFound)
	r.HandleFunc("/{urlStub}", server.HandleStub)
	r.PathPrefix("/").HandlerFunc(server.YouAreLost)

	log.Printf("listening on 6789")
	err = http.ListenAndServe(":6789", r)
	if err != nil {
		log.Printf("%v", err)
	}

	return nil
}

var (
	pageTemplate, lostTemplate *template.Template
)

func init() {

	pageTemplate = template.Must(template.ParseFiles("web/linkit.html"))
	lostTemplate = template.Must(template.ParseFiles("web/youarelost.html"))
}

func (s Server) YouAreLost(w http.ResponseWriter, r *http.Request) {

	err := lostTemplate.Execute(w, nil)
	if err != nil {
		log.Printf("%v", err)
	}
}

type LinkDisplay struct {
	Name  string
	URL   template.HTML
	Notes string
	Added time.Time
}

func (s Server) CategoryFromStub(r *http.Request) (Category, bool, error) {

	urlStub := mux.Vars(r)["urlStub"]

	var cat Category
	err := s.DB.QueryRow("select name, url_stub, passcode, page_top_blurb from category where url_stub = ?", urlStub).Scan(
		&cat.Name, &cat.URLStub, &cat.Passcode, &cat.PageTopBlurb,
	)
	if err == sql.ErrNoRows {
		return cat, false, nil
	}
	if err != nil {
		return cat, false, fmt.Errorf("CategoryFromStub failed to query category with urlStub %s: %w", urlStub, err)
	}

	return cat, true, nil
}

func GoogleAPIKey(db *sql.DB) string {

	var apiKey string
	err := db.QueryRow("select api_key from google limit 1").Scan(&apiKey)
	if err != nil {
		log.Fatalf("GoogleAPIKey unable to get google api key from datbase: %v", err)
	}

	return apiKey
}

func (s Server) LinkExists(cat Category, url string) (bool, error) {

	var x int
	err := s.DB.QueryRow("select 1 from link where category = ? and (upper(url) = ? or upper(url) = ?) limit 1",
		cat.URLStub, strings.ToUpper(url), strings.ToUpper(url)+"/").Scan(&x)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("LinkExists failed to query with url %s: %w", url, err)
	}

	return true, nil
}

func (s Server) AddLink(cat Category, r *http.Request) ([]string, []string, error) {

	var success, failures []string

	if r.Method != http.MethodPost {
		return success, failures, nil
	}

	passcode := r.FormValue("passcode")
	name := r.FormValue("name")
	url := r.FormValue("url")
	notes := r.FormValue("notes")

	if passcode != cat.Passcode {
		return success, append(failures, "The passcode does not match. Link not addeed."), nil
	}

	exists, err := s.LinkExists(cat, url)
	if err != nil {
		return success, failures, fmt.Errorf("AddLink failed to check exists: %w", err)
	}

	if exists {
		return success, append(failures, "That link has already been added."), nil
	}

	threats, err := s.SB.LookupURLs([]string{
		url,
	})
	if err != nil {
		return success, failures, fmt.Errorf("AddLink failed to check URL safety: %w", err)
	}

	if len(threats[0]) != 0 {
		log.Printf("URL unsafe, threats: %v", threats)
		return success, append(failures, "Sorry, that URL is not allowed."), nil
	}

	log.Printf("inserting %s, %s, %s, %s, %s", cat.URLStub, name, url, notes, time.Now().UTC().Format(time.RFC3339))

	_, err = s.DB.Exec("insert into link (category, name, url, notes, added) values (?, ?, ?, ?, ?)",
		cat.URLStub, name, url, notes, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return success, failures, fmt.Errorf("AddLink failed to insert new link: %w", err)
	}

	return append(success, "Your link was added!"), failures, nil
}

func (s Server) HandleStub(w http.ResponseWriter, r *http.Request) {

	cat, found, err := s.CategoryFromStub(r)
	if err != nil {
		log.Printf("HandleStub failed: %v", err)
		http.Error(w, "sorry, we're having technical issues", http.StatusInternalServerError)
		return
	}
	if !found {
		s.YouAreLost(w, r)
		return
	}

	successMsg, failureMsg, err := s.AddLink(cat, r)
	if err != nil {
		log.Printf("HandleStub failed: %v", err)
		http.Error(w, "sorry, we're having technical issues", http.StatusInternalServerError)
		return
	}

	linkList, err := s.LinksByCategory(cat.URLStub)
	if err != nil {
		log.Printf("HandleStub failed to query links: %v", err)
		http.Error(w, "sorry, we're having technical issues", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Links":           linkList,
		"Name":            cat.Name,
		"PageTopBlurb":    template.HTML(cat.PageTopBlurb.String),
		"SuccessMessages": successMsg,
		"FailureMessages": failureMsg,
		"FormValues": map[string]interface{}{
			"name":  r.FormValue("name"),
			"url":   r.FormValue("url"),
			"notes": r.FormValue("notes"),
		},
	}

	err = pageTemplate.Execute(w, data)
	if err != nil {
		log.Printf("%v", err)
	}
}

func (s Server) LinksByCategory(category string) ([]LinkDisplay, error) {

	linkList := []LinkDisplay{}

	rows, err := s.DB.Query("select name, url, notes, added from link where category = ? order by id desc", category)
	if err != nil {
		return linkList, fmt.Errorf("LinksByCategory failed to query category %s: %w", category, err)
	}
	defer rows.Close()

	for rows.Next() {
		var link Link
		err := rows.Scan(&link.Name, &link.URL, &link.Notes, &link.Added)
		if err != nil {
			return linkList, fmt.Errorf("LinksByCategory failed to scan link: %w", err)
		}

		linkList = append(linkList, LinkDisplay{
			Name:  link.Name,
			URL:   template.HTML(link.URL),
			Notes: link.Notes.String,
		})
	}

	return linkList, nil
}
