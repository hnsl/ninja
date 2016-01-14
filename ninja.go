package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/yuin/gopher-lua"
	"golang.org/x/net/websocket"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
)

var turtles = map[turtleID]turtle{}

var kern_version int

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

func atoi(s string) int {
	n, _ := strconv.ParseInt(s, 10, 64)
	return int(n)
}

var web_root_dir string

func main() {
	log.SetPrefix("[ninja]")
	term_ch := make(chan os.Signal, 1)
	signal.Notify(term_ch, os.Interrupt, os.Kill)
	// Start sync.
	go syncGo()
	// Read state.
	if len(os.Args) < 2 {
		panic("arg 1: expect state directory")
	}
	state_dir := os.Args[1]
	loadState(state_dir)
	if len(os.Args) < 3 {
		panic("arg 2: expect web root directory")
	}
	web_root_dir = os.Args[2]
	// Start work manager.
	go workMgrGo()
	// Run lua script and get version.
	log.Printf("running kernel\n")
	kern := lua.NewState()
	err := kern.DoString("is_server = true")
	check(err)
	err = kern.DoString(lua_src_json)
	check(err)
	kern.SetGlobal("JSON", kern.Get(-1))
	err = kern.DoString(lua_src_kernel)
	check(err)
	kern_version = int(lua.LVAsNumber(kern.GetGlobal("version")))
	if kern_version < 1 {
		panic("failed to get global 'version' from kernel")
	}
	log.Printf("kernel version %v ready\n", kern_version)
	log.Printf("starting http server\n")
	// Start HTTP server.
	go goHttpServer()
	// Wait for termination and exit gracefully.
	<-term_ch
	log.Printf("got term, exiting\n")
	workMgrExit()
	os.Exit(0)
}

var root_key = "/72ceda8b"

func goHttpServer() {
	// Start HTTP server.
	http.HandleFunc("/", indexPage)
	http.Handle(root_key+"/", http.StripPrefix(root_key+"/", http.FileServer(http.Dir(web_root_dir))))
	http.HandleFunc(root_key+"/kernel", getKernel)
	http.HandleFunc(root_key+"/version", getVersion)
	http.HandleFunc(root_key+"/report", postReport)
	http.HandleFunc(root_key+"/export", postExport)
	http.Handle(root_key+"/sync", websocket.Handler(wsSync))
	log.Fatal(http.ListenAndServe(":4456", nil))
}

func indexPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		writeRspNotFound(w)
		return
	}
	// TODO: ?
	writeRspNotFound(w)
}

func getKernel(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(lua_src_kernel))
}

func getVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(fmt.Sprintf("%d", kern_version)))
}

func postReport(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	_, err := buf.ReadFrom(r.Body)
	if err != nil {
		return
	}
	//fmt.Printf("incoming raw report: %v\n", buf.String())
	//err := r.ParseMultipartForm(40 * 1000 * 1000)
	var t turtle
	err = json.Unmarshal(buf.Bytes(), &t)
	if err != nil {
		log.Printf("decoding report body failed: %v\n", err)
		return
	}
	fmt.Printf("incoming report: %#v\n", t)
	// Update reported turtle data.
	turtles[t.Label] = t
	syncNotify("turtles/"+string(t.Label), buf.String())
	// Prepare response.
	var rsp bytes.Buffer
	// Decide new work for turtle.
	if t.CurWork != nil {
		fmt.Printf("existing work: %#v\n", *t.CurWork)
	}
	work_rsp := ""
	work_ptr := decideWork(t)
	if work_ptr == nil {
		// Deciding work failed.
		fmt.Printf("failed to decide work\n")
		writeRspInternalError(w)
		return
	}
	work_rsp = *work_ptr
	fmt.Printf("work decision: %s\n\n", work_rsp)
	rsp.WriteString("{")
	rsp.WriteString(work_rsp)
	if t.Version < kern_version && !t.NewKernel {
		rsp.WriteString("new_kernel = ")
		rsp.WriteString(strconv.Quote(lua_src_kernel))
		rsp.WriteString(",")
	}
	rsp.WriteString("}")
	w.Header().Set("Content-Type", "text/plain")
	w.Write(rsp.Bytes())
}

func postExport(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	_, err := buf.ReadFrom(r.Body)
	if err != nil {
		return
	}
	var req exportRequest
	err = json.Unmarshal(buf.Bytes(), &req)
	if err != nil {
		log.Printf("decoding export request failed: %v\n", err)
		return
	}
	log.Printf("got export request: %v\n", req)
	ret := exportItems(req)
	raw_rsp, err := json.Marshal(ret)
	check(err)
	w.Header().Set("Content-Type", "application/json")
	w.Write(raw_rsp)
}

func writeRspNotFound(w http.ResponseWriter) {
	http.Error(w, "Not Found", http.StatusNotFound)
}

func writeRspInternalError(w http.ResponseWriter) {
	http.Error(w, "<h1>Internal Server Error</h1>", http.StatusInternalServerError)
}
