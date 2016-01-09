package main

import (
	"io/ioutil"
	"os"
	"strings"
)

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	raw_b, err := ioutil.ReadFile(os.Args[1])
	check(err)
	raw_s := string(raw_b)
	lines := strings.Split(raw_s, "\n")
	out_lines := make([]string, 0)
	seen := map[string]bool{}
	for _, line := range lines {
		parts := strings.Split(line, `": "`)
		if len(parts) > 1 {
			if seen[parts[0]] {
				continue
			}
			seen[parts[0]] = true
		}
		out_lines = append(out_lines, line)
	}
	ioutil.WriteFile(os.Args[1]+".out", []byte(strings.Join(out_lines, "\n")), 0644)
}
