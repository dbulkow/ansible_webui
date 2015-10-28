package main

import (
	"bufio"
	"flag"
	"fmt"
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

func runAnsible(inventory, playbook, remote string) (string, error) {
	os.Mkdir("jobs", 0700)

	jobdir, err := ioutil.TempDir("jobs", "")
	if err != nil {
		return "", err
	}

	err = ioutil.WriteFile(jobdir+"/inventory", []byte(inventory), 0444)
	if err != nil {
		return "", err
	}

	err = ioutil.WriteFile(jobdir+"/playbook.yml", []byte(playbook), 0444)
	if err != nil {
		return "", err
	}

	ioutil.WriteFile(jobdir+"/remote", []byte("job started by "+remote), 0444)

	logfname := jobdir + "/log"

	f, err := os.OpenFile(logfname, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0444)
	if err != nil {
		return "", err
	}
	defer f.Close()

	curdir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	os.Symlink(curdir+"/roles", jobdir+"/roles")

	cmd := exec.Command("ansible-playbook", "-i", jobdir+"/inventory", jobdir+"/playbook.yml")
	cmd.Stdout = f
	cmd.Stderr = f
	cmd.Dir = curdir

	/*
	 * ansible is made from python, which wants to buffer output.
	 * disable that here by setting the environment variable.
	 */
	env := os.Environ()
	env = append(env, "PYTHONUNBUFFERED=1")
	cmd.Env = env

	err = cmd.Start()
	if err != nil {
		return "", fmt.Errorf("command Start: %v", err)
	}

	go func() {
		err := cmd.Wait()
		if err != nil {
			log.Printf("command finished with error: %v", err)
		}
	}()

	return logfname, nil
}

func requestHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "POST":
		r.ParseForm()

		inventory := r.PostFormValue("inventory")
		playbook := r.PostFormValue("playbook")
		remote := r.RemoteAddr

		logfname, err := runAnsible(inventory, playbook, remote)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			log.Println(err)
			return
		}

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

func serveStatus(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "templates/status.html")
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
	http.HandleFunc("/status", serveStatus)
	log.Fatal(http.ListenAndServe(":"+*port, nil))
}
