package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"time"

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

type Link struct {
	ID       int64
	Category string
	Name     string
	URL      string
	Notes    sql.NullString
	Added    sql.NullString
	Safe     int
}

// .headers on
// .mode columns
// drop table link;
// create table link (id INTEGER PRIMARY KEY AUTOINCREMENT, category TEXT, name TEXT, url TEXT, notes TEXT, added TEXT, safe INTEGER DEFAULT 0);
// insert into link (category,name,url,notes) values ('Hawaii', 'Patrick D Kelly', 'https://instagram.com/phlatphrog', 'taking the scenic route');
// insert into link (category,name,url,notes) values ('Hawaii', 'Ryan Ozawa', 'https://www.instagram.com/hawaii/', 'online social media mastermind');

type Server struct {
	DB *sql.DB
}

func run(args []string, stdout io.Writer) error {

	db, _ := sql.Open("sqlite3", "linkit.db")
	defer db.Close()

	server := Server{
		DB: db,
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
	r.HandleFunc("/hawaii", server.ServePage)
	r.PathPrefix("/").HandlerFunc(server.YouAreLost)

	log.Printf("listening on 8888")
	err := http.ListenAndServe(":8888", r)
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

func (s Server) ServePage(w http.ResponseWriter, r *http.Request) {

	log.Printf("%s", r.URL.Path)

	rows, err := s.DB.Query("select name, url, notes, added from link where category = 'Hawaii' order by id desc")
	if err != nil {
		log.Printf("failed to query link table: %v", err)
	}
	defer rows.Close()

	linkList := []LinkDisplay{}

	for rows.Next() {
		link := Link{}
		err := rows.Scan(&link.Name, &link.URL, &link.Notes, &link.Added)
		if err != nil {
			log.Printf("failed to scan row: %v", err)
			break
		}
		linkList = append(linkList, LinkDisplay{
			Name:  link.Name,
			URL:   template.HTML(link.URL),
			Notes: link.Notes.String,
		})
	}

	data := map[string]interface{}{
		"Links": linkList,
	}

	err = pageTemplate.Execute(w, data)
	if err != nil {
		log.Printf("%v", err)
	}
}
