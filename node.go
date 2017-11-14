package gossip

import (
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"time"

	"gitlab.com/n-canter/graph.git"
)

// Ack structs are sent down a channel on every ack, as well as when a
// multicast is first sent (with FromPort == st.port)
type Ack struct {
	FromPort int
	MsgID    int
	Ticks    int
}

type relayMsg struct {
	body    []byte
	toPorts []int
	ttl     uint
	id      int
	needAck bool
}

type Node struct {
	ip       net.IP
	conn     *net.UDPConn
	port     int
	neigh    []graph.Node
	interval float64
	ttl      uint

	knownMsgIDs map[int]struct{}
	toRelay     []relayMsg
	m           sync.Mutex

	ticks  int
	quitCh chan struct{}

	Feedback chan Ack
}

// Utility functions

func getAddr(ip net.IP, port int) *net.UDPAddr {
	return &net.UDPAddr{
		IP:   ip,
		Port: port,
		Zone: "",
	}
}

func getDestPorts(neigh []graph.Node, recvPort int) []int {
	acceptableNeigh := make([]int, 0, len(neigh))

	for _, v := range neigh {
		if v.Port() != recvPort {
			acceptableNeigh = append(acceptableNeigh, v.Port())
		}
	}

	return acceptableNeigh
}

// Adds a message to the toRelay slice to be sent by us, overriding the Sender
// field accordingly
func (st *Node) addRelayMsg(obj message) bool {
	st.m.Lock()
	defer st.m.Unlock()

	if _, ok := st.knownMsgIDs[obj.ID]; ok {
		return false
	}
	st.knownMsgIDs[obj.ID] = struct{}{}

	toPorts := getDestPorts(st.neigh, obj.Sender)
	if len(toPorts) > 0 {
		needAck := obj.Type == MULTICAST && obj.Sender == st.port
		obj.Sender = st.port

		st.toRelay = append(st.toRelay, relayMsg{
			body:    toJSON(&obj),
			toPorts: toPorts,
			ttl:     st.ttl,
			id:      obj.ID,
			needAck: needAck,
		})
	}

	return true
}

// Communication functions

func receive(conn *net.UDPConn, msgs chan *message, stopRecvCh chan struct{}) {
	timeout := 100 * time.Millisecond
	messagebytes := make([]byte, 1024)

	for {
		select {
		case <-stopRecvCh:
			return
		default:
		}

		err := conn.SetReadDeadline(time.Now().Add(timeout))
		if err != nil {
			panic(err)
		}

		nbytes, err := conn.Read(messagebytes)
		if err != nil {
			if e, ok := err.(net.Error); !ok || !e.Timeout() {
				panic(err)
			} else {
				continue
			}
		}

		obj := fromJSON(messagebytes[:nbytes])

		select {
		case msgs <- obj: // empty
		case <-stopRecvCh:
			return
		}
	}
}

func (st *Node) processRecvdMsg(obj *message) {
	if !st.addRelayMsg(*obj) {
		return
	}

	switch obj.Type {
	case MULTICAST:
		ack := message{
			ID:     obj.ID + st.port,
			Sender: st.port,
			Origin: st.port,
			Type:   NOTIFICATION,
			Data:   strconv.Itoa(obj.ID),
		}

		st.addRelayMsg(ack)

	case NOTIFICATION:
		msgid, err := strconv.Atoi(obj.Data)
		if err != nil {
			panic(err)
		}

		if st.Feedback != nil {
			st.Feedback <- Ack{
				MsgID:    msgid,
				FromPort: obj.Origin,
				Ticks:    st.ticks,
			}
		}

	default:
		panic(fmt.Sprintf("invalid msg type %d", obj.Type))
	}
}

func (st *Node) getRandomMsg() (body []byte, toAddr *net.UDPAddr, ack *Ack) {
	st.m.Lock()
	defer st.m.Unlock()

	if len(st.toRelay) == 0 {
		return nil, nil, nil
	}

	i := rand.Intn(len(st.toRelay))
	msgref := &st.toRelay[i]

	body = msgref.body

	toNeigh := rand.Intn(len(msgref.toPorts))
	toAddr = getAddr(st.ip, msgref.toPorts[toNeigh])

	if msgref.needAck {
		msgref.needAck = false
		ack = &Ack{
			MsgID:    msgref.id,
			FromPort: st.port,
			Ticks:    st.ticks,
		}
	}

	if msgref.ttl == 1 {
		st.toRelay = append(st.toRelay[:i], st.toRelay[i+1:]...)
	} else if msgref.ttl > 1 {
		msgref.ttl--
	}

	return
}

func (st *Node) writeRandomMsg() {
	timeout := 100 * time.Millisecond

	body, addr, ack := st.getRandomMsg()
	if body == nil {
		return
	}

	if ack != nil && st.Feedback != nil {
		st.Feedback <- *ack
	}

	err := st.conn.SetWriteDeadline(time.Now().Add(timeout))
	if err != nil {
		panic(err)
	}

	_, err = st.conn.WriteToUDP(body, addr)
	if err != nil {
		if e, ok := err.(net.Error); !ok || !e.Timeout() {
			panic(err)
		}
	}
}

func (st *Node) mainLoop() {
	ticker := time.NewTicker(time.Duration(float64(time.Second) * st.interval))
	defer ticker.Stop()

	msgs := make(chan *message)
	stopRecvCh := make(chan struct{})
	go receive(st.conn, msgs, stopRecvCh)

	defer func() {
		stopRecvCh <- struct{}{}

		if st.Feedback != nil {
			close(st.Feedback)
		}

		if err := st.conn.Close(); err != nil {
			panic(err)
		}
	}()

	for {
		select {
		case msg := <-msgs:
			st.processRecvdMsg(msg)

		case <-ticker.C:
			st.ticks++
			st.writeRandomMsg()

		case <-st.quitCh:
			return
		}
	}
}

func RunNode(ip net.IP, node graph.Node, neigh []graph.Node, interval float64, ttl uint, feedback bool) *Node {
	conn, err := net.ListenUDP("udp4", getAddr(ip, node.Port()))
	if err != nil {
		panic(err)
	}

	n := &Node{
		conn:     conn,
		ip:       ip,
		port:     node.Port(),
		neigh:    neigh,
		interval: interval,
		ttl:      ttl,

		knownMsgIDs: make(map[int]struct{}),
		toRelay:     make([]relayMsg, 0),
		ticks:       0,
		quitCh:      make(chan struct{}, 1),
	}
	if feedback {
		n.Feedback = make(chan Ack)
	}

	go n.mainLoop()
	return n
}

func (st *Node) Send(id int, data string) bool {
	return st.addRelayMsg(message{
		ID:     id,
		Data:   data,
		Sender: st.port,
		Origin: st.port,
		Type:   MULTICAST,
	})
}

func (st *Node) Port() int {
	return st.port
}

func (st *Node) Stop() {
	st.quitCh <- struct{}{}
}
