package main

import (
	"encoding/json"
	"github.com/Manishearth/cs733/assignment3/raft"
	"io/ioutil"
	"log"
	"net/rpc"
	"os"
	"os/exec"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Not enough arguments, please pass the path of the config file")
	}
	path := os.Args[1]
	file, e := ioutil.ReadFile(path)
	if e != nil {
		log.Fatal("")
	}
	var set settings
	json.Unmarshal(file, &set)

	EventCh := make(chan raft.Signal, 1000)
	commit := make(chan raft.LogEntry, 100)
	logarr := make([]raft.LogEntry, 0)
	network := raft.NetCommunicationHelper{
		Id:      set.Id,
		Hosts:   set.Hosts,
		Ports:   set.Ports,
		Handler: raft.NetRPCHandler{EventCh},
	}
	server := raft.RaftServer{
		Id:          set.Id,
		CommitCh:    commit,
		EventCh:     EventCh,
		Network:     network,
		Log:         logarr,
		Term:        0,
		VotedFor:    -1,
		CommitIndex: -1,
		LastApplied: 0,
		N:           uint(len(set.Hosts)),
		Leader:      0,
		Persistence: FilePersistence{},
	}

	var persist persistentraft
	file, e = ioutil.ReadFile("./raft_persistence.json")
	if e == nil {
		json.Unmarshal(file, &persist)
		server.Log = persist.Log
		server.Term = persist.Term
		server.VotedFor = persist.VotedFor
		server.CommitIndex = persist.CommitIndex
		server.LastApplied = persist.LastApplied
	}
	ServeClients(EventCh)
	go server.Loop()
	for c := range commit {
		// Reset working dir
		os.Chdir(set.Path)
		st := string(c.Data)
		cmd := exec.Command("bash", "-c", st)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		log.Printf("Executing %v\n", st)
		e := cmd.Run()
		if e != nil {
			log.Printf("Encountered error %v\n", e)
		}
	}
}

type settings struct {
	Id    uint
	Hosts []string
	Ports []uint
	Path  string
}

type persistentraft struct {
	Log         []raft.LogEntry
	Term        uint
	VotedFor    int
	CommitIndex int
	LastApplied uint
}

type ClientAppendHandler struct {
	EventCh chan<- raft.Signal
}

type AppendReply struct {
	Queued bool
	Leader uint
}

func (c *ClientAppendHandler) ClientAppend(data *string, reply *raft.ClientAppendResponse) error {
	ack := make(chan raft.ClientAppendResponse, 1)
	c.EventCh <- raft.ClientAppendEvent{Data: (raft.Data(*data)), Ack: ack}
	*reply = <-ack
	return nil
}

func ServeClients(ch chan<- raft.Signal) {
	handler := ClientAppendHandler{EventCh: ch}
	raft.Register()
	rpc.Register(&handler)
}

type FilePersistence struct{}

func (p FilePersistence) Save(r *raft.RaftServer) {
	persistence := persistentraft{
		Log:         r.Log,
		Term:        r.Term,
		VotedFor:    r.VotedFor,
		CommitIndex: r.CommitIndex,
		LastApplied: r.LastApplied,
	}

	bytes, e := json.Marshal(persistence)
	if e != nil {
		log.Fatal("There was trouble converting data to JSON")
	}
	file, e := os.Create("./raft_persistence.json")
	defer file.Close()
	if e != nil {
		log.Printf("There was trouble syncing with persistent storage")
		return
	}
	_, e = file.Write(bytes)
	if e != nil {
		log.Printf("There was trouble syncing with persistent storage")
		return
	}
	file.Sync()
	// Todo: Add resilience to deletion in the middle of a disk write
}
