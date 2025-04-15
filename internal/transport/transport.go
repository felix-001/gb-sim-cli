package transport

import (
	"math/rand"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/jart/gosip/sip"
	"github.com/lzh2nix/gb28181Simulator/internal/config"
	"github.com/qiniu/x/xlog"
)

const sipMaxPacketSize = 1500

type Transport struct {
	Conn    *net.UDPConn
	TcpConn *net.TCPConn
	Recv    chan *sip.Msg
	Send    chan *sip.Msg
}

func startTcpSip(xlog *xlog.Logger, remoteAddr string, cfg *config.Config) (*Transport, error) {
	rAddr, err := net.ResolveTCPAddr("tcp", remoteAddr)
	if err != nil {
		return nil, err
	}
	net, err := net.DialTCP("tcp", nil, rAddr)
	if err != nil {
		xlog.Errorf("err = %#v", err.Error())
		return nil, err
	}
	recvChan := make(chan *sip.Msg)
	sendChan := make(chan *sip.Msg, 1000)
	go tcpsend(xlog, net, sendChan, cfg)
	go tcprecv(xlog, net, recvChan, cfg)
	tr := &Transport{
		TcpConn: net,
		Recv:    recvChan,
		Send:    sendChan,
	}

	return tr, nil
}

func StartSip(xlog *xlog.Logger, remoteAddr string, transport string, cfg *config.Config) (*Transport, error) {
	if transport == "tcp" {
		return startTcpSip(xlog, remoteAddr, cfg)
	}
	rAddr, err := net.ResolveUDPAddr("udp", remoteAddr)
	if err != nil {
		return nil, err
	}
	net, err := net.DialUDP(transport, nil, rAddr)
	if err != nil {
		xlog.Errorf("err = %#v", err.Error())
		return nil, err
	}
	recvChan := make(chan *sip.Msg)
	sendChan := make(chan *sip.Msg, 1000)
	go send(xlog, net, sendChan, cfg)
	go recv(xlog, net, recvChan, cfg)
	tr := &Transport{
		Conn: net,
		Recv: recvChan,
		Send: sendChan,
	}

	return tr, nil
}

func recv(xlog *xlog.Logger, conn *net.UDPConn, output chan *sip.Msg, cfg *config.Config) {

	for {
		buf := make([]byte, sipMaxPacketSize)
		conn.SetReadDeadline(time.Now().Add(time.Second * 5))
		n, err := conn.Read(buf)
		if n == 0 || err != nil {
			continue
		}
		msg, err := sip.ParseMsg(buf[:n])
		if err != nil {
			xlog.Errorf("parse msg failed, err =%v", err)
			continue
		}
		if cfg.DetailLog {
			xlog.Debug("recv msg \n", msg)
		}
		output <- msg
	}
}

func send(xlog *xlog.Logger, conn *net.UDPConn, input chan *sip.Msg, cfg *config.Config) {

	msgs := []*sip.Msg{} // 定义一个空的 msgs 变量，类型为 []*sip.Ms

	for m := range input {
		if cfg.DetailLog {
			xlog.Debug("send msg \n", m)
		}
		//log.Printf("send addr: %p msg type: %s", m, m.Method)
		// 初始化随机数种子
		rand.Seed(time.Now().UnixNano())
		// 生成1到10的随机数
		randomNum := rand.Intn(10) + 1

		if len(msgs) < 5 {
			msgs = append(msgs, m)
		}

		if randomNum > 5 && len(msgs) == 5 {
			// 这里添加大于5时要执行的逻辑
			s := m.String()
			for i := 0; i < len(msgs); i++ {
				s += msgs[i].String()
			}
			xlog.Debugf("随机数 %d 大于 5，执行特定逻辑, len: %d", randomNum, len(s))
			if _, err := conn.Write([]byte(s)); err != nil {
				xlog.Errorf("send msg failed, err = #v", err)
			}
		} else {
			if _, err := conn.Write([]byte(m.String())); err != nil {
				xlog.Errorf("send msg failed, err = #v", err)
			}

		}
	}

}

func tcpsend(xlog *xlog.Logger, conn *net.TCPConn, input chan *sip.Msg, cfg *config.Config) {

	msgs := []*sip.Msg{} // 定义一个空的 msgs 变量，类型为 []*sip.Ms

	for m := range input {
		if cfg.DetailLog {
			xlog.Debug("send msg \n", m)
		}
		//log.Printf("send addr: %p msg type: %s", m, m.Method)
		// 初始化随机数种子
		rand.Seed(time.Now().UnixNano())
		// 生成1到10的随机数
		randomNum := rand.Intn(10) + 1

		if len(msgs) < 5 && !m.IsResponse() && m.CSeqMethod == sip.MethodMessage && msgType(m) == "Catalog" {
			msgs = append(msgs, m)
		}

		if randomNum > 10 && !m.IsResponse() && m.CSeqMethod == sip.MethodMessage && msgType(m) == "Catalog" && len(msgs) == 5 {
			// 这里添加大于5时要执行的逻辑
			s := m.String()
			for i := 0; i < len(msgs); i++ {
				s += msgs[i].String()
			}
			xlog.Debugf("随机数 %d 大于 5，执行特定逻辑, len: %d", randomNum, len(s))
			if _, err := conn.Write([]byte(s)); err != nil {
				xlog.Errorf("send msg failed, err = #v", err)
			}
		} else {
			if _, err := conn.Write([]byte(m.String())); err != nil {
				xlog.Errorf("send msg failed, err = #v", err)
			}

		}
	}

}
func tcprecv(xlog *xlog.Logger, conn *net.TCPConn, output chan *sip.Msg, cfg *config.Config) {

	for {
		buf := make([]byte, sipMaxPacketSize)
		conn.SetReadDeadline(time.Now().Add(time.Second * 5))
		n, err := conn.Read(buf)
		if n == 0 || err != nil {
			continue
		}
		msg, err := sip.ParseMsg(buf[:n])
		if err != nil {
			xlog.Errorf("parse msg failed, err =%v", err)
			continue
		}
		if cfg.DetailLog {
			xlog.Debug("recv msg \n", msg)
		}
		output <- msg
	}
}

var msgTypeRegexp = regexp.MustCompile(`<CmdType>([\w]+)</CmdType>`)

func msgType(m *sip.Msg) string {
	if len(m.Payload.Data()) != 0 && m.Payload.ContentType() == "Application/MANSCDP+xml" {
		cmdType := msgTypeRegexp.FindString(string(m.Payload.Data()))
		cmdType = strings.TrimPrefix(cmdType, "<CmdType>")
		return strings.TrimSuffix(cmdType, "</CmdType>")
	}
	return "unknow"
}
