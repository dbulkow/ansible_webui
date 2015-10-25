package main

import (
	"bufio"
	"flag"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"text/template"
)

func readdir(dir string, check func(os.FileInfo) bool, suffix string) []string {
	var list []string

	d, err := os.Open(dir)
	if err != nil {
		return list
	}
	defer d.Close()

	fi, err := d.Readdir(-1)
	if err != nil {
		return list
	}

	for _, f := range fi {
		log.Println("checking", f.Name())
		if check(f) {
			if suffix != "" && !strings.HasSuffix(f.Name(), suffix) {
				continue
			}
			list = append(list, strings.TrimSuffix(f.Name(), suffix))
		}
	}

	sort.Strings(list)

	return list
}

func checkDir(f os.FileInfo) bool {
	if f.Mode().IsDir() {
		return true
	}

	return false
}

func checkFile(f os.FileInfo) bool {
	if f.Mode().IsRegular() {
		return true
	}

	return false
}

func readFile(file string) []string {
	var lines []string

	f, err := os.Open(file)
	if err != nil {
		log.Println(err)
		return lines
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	return lines
}

func requestHandler(w http.ResponseWriter, r *http.Request) {
	t, err := template.ParseFiles("templates/ansible.html")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		log.Println(err)
	}

	ansible_details := &struct {
		Machines  []string
		Playbooks []string
		Roles     []string
	}{
		Machines:  readFile("machines"),
		Playbooks: readdir("playbooks", checkFile, ".yml"),
		Roles:     readdir("roles", checkDir, ""),
	}

	err = t.Execute(w, ansible_details)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		log.Println(err)
	}
}

func serveAssets(w http.ResponseWriter, r *http.Request) {
	log.Println("serving", r.URL.Path[1:])
	http.ServeFile(w, r, r.URL.Path[1:])
}

func main() {
	var port = flag.String("port", "", "HTTP service address (.e.g. 8080)")

	flag.Parse()

	if *port == "" {
		flag.Usage()
		return
	}

	http.HandleFunc("/", requestHandler)
	http.HandleFunc("/playbooks/", serveAssets)
	http.HandleFunc("/assets/", serveAssets)
	log.Fatal(http.ListenAndServe(":"+*port, nil))
}
