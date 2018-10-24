package main

import (
	"log"
	"fmt"
	"io/ioutil"
	"net/http"
	"html/template"
	"regexp"
	"errors"
	"sync"
	"crypto/subtle"
	"github.com/satori/go.uuid"
)

type authenticationMiddleware struct {
	wrappedHandler http.Handler
}

type Client struct {
	loggedIn bool
}

type Page struct {
	Title string
	Body []byte
}

type helloWorldHandler struct {

}

func (h helloWorldHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "Hello World")
}

var templates = template.Must(template.ParseFiles("templates/edit.html", "templates/view.html", "templates/login.html", "templates/failed-login.html"))
var validPath = regexp.MustCompile("^/(edit|save|view|login)/([a-zA-Z0-9]+)$")
var sessionStore map[string]Client
var storageMutex sync.RWMutex


func loadPage(title string) (*Page, error) {
	filename := title + ".txt"
	body, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return &Page{Title: title, Body: body}, nil
}

func getTitle(w http.ResponseWriter, r *http.Request) (string, error) {
	m := validPath.FindStringSubmatch(r.URL.Path)
	if m == nil {
		http.NotFound(w, r)
		return "", errors.New("Invalid Page Title")
	}
	return m[2], nil
}

func (p *Page) save() error {
	filename := p.Title + ".txt"
	return ioutil.WriteFile(filename, p.Body, 0600)
}

func makeHandler(fn func (http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m := validPath.FindStringSubmatch(r.URL.Path)
		if m == nil {
			http.NotFound(w, r)
			return
		}
		fn(w, r, m[2])
	}
}

func viewHandler(w http.ResponseWriter, r *http.Request, title string) {
	p, err := loadPage(title)
	if err != nil {
		http.Redirect(w, r, "/edit/"+title, http.StatusFound)
		return
	}
	renderTemplate(w, "view", p)
}

func editHandler(w http.ResponseWriter, r *http.Request, title string) {
	p, err := loadPage(title)
	if err != nil {
		p = &Page{Title: title}
	}
	renderTemplate(w, "edit", p)
}

func saveHandler(w http.ResponseWriter, r *http.Request, title string) {
	body := r.FormValue("body")
	p := &Page{Title: title, Body: []byte(body)}
	err := p.save()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/view/"+title, http.StatusFound)
}

func renderTemplate(w http.ResponseWriter, tmpl string, p *Page) {
	err := templates.ExecuteTemplate(w, tmpl+".html", p)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session")
	if err != nil {
		if err != http.ErrNoCookie {
			fmt.Fprint(w, err)
			return
		} else {
			err = nil
		}
	}
	var present bool
	var client Client
	if cookie != nil {
		storageMutex.RLock()
		client, present = sessionStore[cookie.Value]
		storageMutex.RUnlock()
	} else {
		present = false
	}

	if present == false {
		u, err := uuid.NewV4()
		if err != nil {
			return
		}
		uuidValue := u.String()
		cookie = &http.Cookie {
			Name: "session",
			Value: uuidValue,
		}
		client = Client{false}
		storageMutex.Lock()
		sessionStore[cookie.Value] = client
		storageMutex.Unlock()
	}

	http.SetCookie(w, cookie)

	err = r.ParseForm()
	if err != nil {
		fmt.Fprint(w, err)
		return
	}

	if subtle.ConstantTimeCompare([]byte(r.FormValue("password")), []byte("password123")) == 1 {
		client.loggedIn = true
		fmt.Fprintln(w, "Thank you for logging in.")
		storageMutex.Lock()
		sessionStore[cookie.Value] = client
		storageMutex.Unlock()
	} else {
		p := &Page{Title: "Failed Login Attempt", Body: nil}
		renderTemplate(w, "failed-login", p)
	}
}

func (h authenticationMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session")
	if err != nil {
		if err != http.ErrNoCookie {
			fmt.Fprint(w, err)
			return
		} else {
			err = nil
		}
	}

	var present bool
	var client Client
	if cookie != nil {
		storageMutex.RLock()
		client, present = sessionStore[cookie.Value]
		storageMutex.RUnlock()
	} else {
		present = false
	}

	if present == false {
		u, err := uuid.NewV4()
		if err != nil {
			return
		}
		uuidValue := u.String()
		cookie = &http.Cookie{
			Name: "session",
			Value: uuidValue,
		}
		client = Client{false}
		storageMutex.Lock()
		sessionStore[cookie.Value] = client
		storageMutex.Unlock()
	}

	http.SetCookie(w, cookie)
	if client.loggedIn == false {
		p := &Page{Title: "Login to Marketplace", Body: nil}
	        renderTemplate(w, "login", p)
		return
	}

	if client.loggedIn == true {
		h.wrappedHandler.ServeHTTP(w, r)
		return
	}
}

func authenticate(h http.Handler) authenticationMiddleware {
	return authenticationMiddleware{h}
}

func main() {
	sessionStore = make(map[string]Client)
	http.Handle("/login/", authenticate(helloWorldHandler{}))
	http.HandleFunc("/home/", loginHandler)
	http.HandleFunc("/view/", makeHandler(viewHandler))
	http.HandleFunc("/edit/", makeHandler(editHandler))
	http.HandleFunc("/save/", makeHandler(saveHandler))
	log.Fatal(http.ListenAndServe(":8080", nil))
}
