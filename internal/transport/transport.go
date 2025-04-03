package transport

import (
	"bytes"
	"log"
	"net"
	"time"

	"github.com/jart/gosip/sip"
	"github.com/qiniu/x/xlog"
)

const sipMaxPacketSize = 15000

type Transport struct {
	Conn *net.TCPConn
	Recv chan *sip.Msg
	Send chan *sip.Msg
	Quit chan bool
}

func StartSip(xlog *xlog.Logger, remoteAddr string, transport string) (*Transport, error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", remoteAddr)
	if err != nil {
		return nil, err
	}
	net, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		xlog.Errorf("err = %#v", err.Error())
		return nil, err
	}
	recvChan := make(chan *sip.Msg, 1000)
	sendChan := make(chan *sip.Msg, 1000)
	quitChan := make(chan bool)
	go send(xlog, net, sendChan, quitChan)
	go recv(xlog, net, recvChan, quitChan)
	tr := &Transport{
		Conn: net,
		Recv: recvChan,
		Send: sendChan,
		Quit: quitChan,
	}

	return tr, nil
}

func recv(xlog *xlog.Logger, conn *net.TCPConn, output chan *sip.Msg, quit chan bool) {

	for {
		select {
		case <-quit:
			log.Println("exit sip recv routine")
			return
		default:
		}

		buf := make([]byte, sipMaxPacketSize)
		conn.SetReadDeadline(time.Now().Add(time.Second * 5))
		n, err := conn.Read(buf)
		if n == 0 || err != nil {
			//log.Println(err, n, "read from conn failed")
			continue
		}
		if bytes.Count(buf[:n], []byte("Content-Length:")) > 1 {
			messageStart := bytes.Index(buf[:n], []byte("MESSAGE"))
			if messageStart != -1 {
				if messageStart < n {
					firstPart := buf[:messageStart]
					firstMsg, err := sip.ParseMsg(firstPart)
					if err != nil {
						log.Println("parse first msg failed, err =", err, "msg =", string(firstPart))
						continue
					}
					output <- firstMsg
					secondPart := buf[messageStart:n]
					secondMsg, err := sip.ParseMsg(secondPart)
					if err != nil {
						log.Println("parse second msg failed, err =", err, "msg =", string(secondPart))
						continue
					}
					output <- secondMsg
				}

			}
		}

		msg, err := sip.ParseMsg(buf[:n])
		if err != nil {
			//xlog.Errorf("parse msg failed, err =%v", err)
			//log.Println("parse msg failed, err =", err)
			continue
		}
		//log.Println("recv msg \n", msg)
		output <- msg
	}
}

func send(xlog *xlog.Logger, conn *net.TCPConn, input chan *sip.Msg, quit chan bool) {

	for m := range input {
		select {
		case <-quit:
			log.Println("exit sip send routine")
			return
		default:
		}
		//xlog.Debug("send msg \n", m)
		//log.Printf("send addr: %p msg type: %s", m, m.Method)
		if _, err := conn.Write([]byte(m.String())); err != nil {
			xlog.Errorf("send msg failed, err = #v", err)
		}
	}

}
