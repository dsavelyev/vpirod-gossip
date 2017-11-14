package main

import (
	"flag"
	"fmt"
	"math/rand"
	"net"
	"os"
	"time"

	"gitlab.com/n-canter/graph.git"
	"gossip"
)

func main() {
	rand.Seed(time.Now().UnixNano())

	n := flag.Int("nodes", 10, "number of nodes")
	mindeg := flag.Int("mindegree", 2, "min degree")
	maxdeg := flag.Int("maxdegree", 3, "max degree")
	ip := flag.String("ip", "127.42.42.42", "IP")
	interval := flag.Float64("interval", 1.0, "interval (secs)")
	ttl := flag.Uint("ttl", 5, "TTL (0 means infinity)")

	flag.Parse()

	gr := graph.Generate(*n, *mindeg, *maxdeg, 9000)
	ourIP := net.ParseIP(*ip)

	nodes := make([]*gossip.Node, *n)

	for i := 0; i < *n; i++ {
		node, ok1 := gr.GetNode(i)
		neigh, ok2 := gr.Neighbors(i)
		if !ok1 || !ok2 {
			panic("GetNode / Neighbors")
		}

		nodes[i] = gossip.RunNode(ourIP, node, neigh, *interval, *ttl, i == 0)
	}

	nodes[0].Send(100500, "lol no generics")

	a := -1
	b := -1
	ackedNodes := make(map[int]struct{})

	for ack := range nodes[0].Feedback {
		ackedNodes[ack.FromPort] = struct{}{}

		if len(ackedNodes)%50 == 0 {
			fmt.Fprintf(os.Stderr, "%d\t\r", len(ackedNodes))
		}

		if ack.FromPort == nodes[0].Port() {
			a = ack.Ticks
		}

		if len(ackedNodes) == int(*n) {
			for _, v := range nodes {
				v.Stop()
			}
			b = ack.Ticks
		}
	}

	fmt.Printf("%d\n", b-a)
}
