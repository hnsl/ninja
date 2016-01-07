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
	"strconv"
)

var turtles = map[turtleID]turtle{}

var kern_version int

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	log.SetPrefix("[ninja]")
	// Start sync.
	go syncGo()
	// Read state.
	if len(os.Args) < 2 {
		panic("arg 1: expect state directory")
	}
	state_dir := os.Args[1]
	loadState(state_dir)
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
	goHttpServer()
}

func goHttpServer() {
	// Start HTTP server.
	key := "/72ceda8b"
	http.HandleFunc("/", indexPage)
	http.HandleFunc(key+"/kernel", getKernel)
	http.HandleFunc(key+"/version", getVersion)
	http.HandleFunc(key+"/report", postReport)
	http.Handle(key+"/sync", websocket.Handler(wsSync))
	log.Fatal(http.ListenAndServe(":4456", nil))
}

func indexPage(w http.ResponseWriter, r *http.Request) {

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
		log.Printf("reading report body failed: %v\n", err)
		return
	}
	fmt.Printf("incoming raw report: %v\n", buf.String())
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

func writeRspInternalError(w http.ResponseWriter) {
	http.Error(w, "<h1>Internal Server Error</h1>", http.StatusInternalServerError)
}
