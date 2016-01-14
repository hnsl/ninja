package main

import (
	"bytes"
	"github.com/google/btree"
	"golang.org/x/net/websocket"
	"log"
	"strconv"
)

type syncNotifyReq struct {
	key  string
	data string
}

type kdVersion struct {
	seq  int64
	key  string
	data string
}

func (this kdVersion) Less(than btree.Item) bool {
	return this.seq < than.(kdVersion).seq
}

type syncRefreshReq struct {
	last_seq int64
	rsp_ch   chan syncRefreshRsp
}

type syncRefreshRsp struct {
	seq    int64
	update map[string]string
	wchan  chan struct{}
}

var syncChan = make(chan interface{}, 16)

func syncGo() {
	seq_next := int64(1)
	dseq := map[string]int64{}
	dlog := btree.New(8)
	cur_wchan := make(chan struct{}, 0)
	for {
		req := <-syncChan
		switch req := req.(type) {
		case syncNotifyReq:
			item_seq := dseq[req.key]
			if item_seq > 0 {
				i := dlog.Get(kdVersion{seq: item_seq})
				cur := i.(kdVersion)
				if cur.data == req.data {
					continue
				}
				dlog.Delete(kdVersion{seq: item_seq})
			}
			dseq[req.key] = seq_next
			dlog.ReplaceOrInsert(kdVersion{seq: seq_next, key: req.key, data: req.data})
			seq_next++
			close(cur_wchan)
			cur_wchan = make(chan struct{}, 0)
		case syncRefreshReq:
			update := map[string]string{}
			dlog.AscendGreaterOrEqual(kdVersion{seq: req.last_seq}, func(i btree.Item) bool {
				kd := i.(kdVersion)
				update[kd.key] = kd.data
				return true
			})
			req.rsp_ch <- syncRefreshRsp{
				seq:    seq_next,
				update: update,
				wchan:  cur_wchan,
			}
		}
	}
}

func wsSync(ws *websocket.Conn) {
	log.Printf("sync: started")
	// Start go routine that reads ok's.
	ws_ok_ch := make(chan struct{}, 1)
	go func() {
		defer close(ws_ok_ch)
		for {
			var ok_rsp []byte
			err := websocket.Message.Receive(ws, &ok_rsp)
			if err != nil || !bytes.Equal(ok_rsp, []byte("ok")) {
				log.Printf("sync: ended %v", err)
				return
			}
			ws_ok_ch <- struct{}{}
		}
	}()
	last_seq := int64(0)
	for {
		req := syncRefreshReq{last_seq: last_seq, rsp_ch: make(chan syncRefreshRsp, 1)}
		syncChan <- req
		rsp := <-req.rsp_ch
		// Generate response to client.
		var payload bytes.Buffer
		payload.Write([]byte("{"))
		first := true
		for key, value := range rsp.update {
			if first {
				first = false
			} else {
				payload.Write([]byte(",\n"))
			}
			payload.Write([]byte(strconv.Quote(key)))
			payload.Write([]byte(": "))
			payload.Write([]byte(value))
		}
		payload.Write([]byte("}"))
		err := websocket.Message.Send(ws, payload.String())
		if err != nil {
			return
		}
		// Wait for ok to throttle writes.
		if _, ok := <-ws_ok_ch; !ok {
			// Websocket was closed.
			return
		}
		// Wait for update.
		// TODO: Cancel wait if websocket is killed.
		last_seq = rsp.seq
		select {
		case <-ws_ok_ch:
			// Websocket was closed or got invalid ok.
			return
		case <-rsp.wchan:
			// Updates are available.
		}
	}
}

func syncNotify(key string, data string) {
	syncChan <- syncNotifyReq{key: key, data: data}
}
