package main

import(
    "fmt"
    "log"
    "bytes"
    "net/http"
    "github.com/yuin/gopher-lua"
)

var kern_version int

func check(err error) {
    if err != nil {
        panic(err)
    }
}

func main() {
    log.SetPrefix("[ninja]")
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
	http.HandleFunc(key + "/kernel", getKernel)
	http.HandleFunc(key + "/version", getVersion)
	http.HandleFunc(key + "/report", postReport)
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
    w.Header().Set("Content-Type", "text/plain")
    w.Write([]byte(fmt.Sprintf("%d", kern_version)))
    var buf bytes.Buffer
    _, err := buf.ReadFrom(r.Body)
    if err != nil {
        log.Printf("reading report body failed: %v\n", err)
        return
    }
    //err := r.ParseMultipartForm(40 * 1000 * 1000)
    fmt.Printf("incoming report: %v", buf.String())
}
