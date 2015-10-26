package main

import (
	"bufio"
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
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
	switch r.Method {
	case "POST":
		r.ParseForm()

		dir, err := ioutil.TempDir("jobs", "")
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			log.Println(err)
			return
		}

		err = ioutil.WriteFile(dir+"/inventory", []byte(r.PostFormValue("inventory")), 0444)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			log.Println(err)
			return
		}

		err = ioutil.WriteFile(dir+"/playbook.yml", []byte(r.PostFormValue("playbook")), 0444)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			log.Println(err)
			return
		}

		ioutil.WriteFile(dir+"/remote", []byte("job started by "+r.RemoteAddr), 0444)

		logfname := dir + "/log"

		f, err := os.OpenFile(logfname, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0444)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			log.Println(err)
			return
		}
		defer f.Close()

		curdir, err := os.Getwd()
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			log.Println(err)
			return
		}

		os.Symlink(curdir+"/roles", dir+"/roles")

		cmd := exec.Command("ansible-playbook", "-i", dir+"/inventory", dir+"/playbook.yml")
		cmd.Stdout = f
		cmd.Stderr = f
		cmd.Dir = curdir

		err = cmd.Start()
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			log.Println("command", err)
			return
		}

		go func() {
			cmd.Wait()
		}()

		logfile_details := &struct {
			Playbook string
			Logfile  string
		}{
			Playbook: r.PostFormValue("playbook_selection"),
			Logfile:  logfname,
		}

		t, err := template.ParseFiles("templates/logfile.html")
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			log.Println(err)
			return
		}

		err = t.Execute(w, logfile_details)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			log.Println(err)
			return
		}

	case "GET":
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
}

func serveAssets(w http.ResponseWriter, r *http.Request) {
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
	http.HandleFunc("/assets/", serveAssets)
	http.HandleFunc("/jobs/", serveAssets)
	http.HandleFunc("/playbooks/", serveAssets)
	log.Fatal(http.ListenAndServe(":"+*port, nil))
}
