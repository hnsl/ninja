package main

import (
	"bytes"
	"github.com/google/btree"
	"golang.org/x/net/websocket"
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
			dlog.Delete(kdVersion{seq: dseq[req.key]})
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
		var ok_rsp []byte
		err = websocket.Message.Receive(ws, &ok_rsp)
		if err != nil {
			return
		}
		// Wait for update.
		// TODO: Cancel wait if websocket is killed.
		last_seq = rsp.seq
		<-rsp.wchan
	}
}

func syncNotify(key string, data string) {
	syncChan <- syncNotifyReq{key: key, data: data}
}
