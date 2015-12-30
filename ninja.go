package main

import(
    "log"
    "net/http"
    "github.com/yuin/gopher-lua"
)

var startup_lua string

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
    vers := lua.LVAsNumber(kern.GetGlobal("version"))
    if vers < 1 {
        panic("failed to get global 'version' from kernel")
    }
    log.Printf("kernel version %d ready\n", vers)
    log.Printf("starting http server\n")
    // Start HTTP server.
    goHttpServer()
}

func goHttpServer() {
	// Start HTTP server.
    key := "/72ceda8b"
	http.HandleFunc("/", indexPage)
	http.HandleFunc(key + "/script", getScript)
	http.HandleFunc(key + "/version", getVersion)
	http.HandleFunc(key + "/report", postReport)
	log.Fatal(http.ListenAndServe(":4456", nil))
}

func indexPage(w http.ResponseWriter, r *http.Request) {

}

func getScript(w http.ResponseWriter, r *http.Request) {

}

func getVersion(w http.ResponseWriter, r *http.Request) {

}

func postReport(w http.ResponseWriter, r *http.Request) {

}
