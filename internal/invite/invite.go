package invite

import (
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jart/gosip/sdp"
	"github.com/jart/gosip/sip"
	"github.com/jart/gosip/util"
	"github.com/lzh2nix/gb28181Simulator/internal/config"
	"github.com/lzh2nix/gb28181Simulator/internal/streams/packet"
	"github.com/lzh2nix/gb28181Simulator/internal/transport"
	"github.com/lzh2nix/gb28181Simulator/internal/version"
	"github.com/qiniu/x/xlog"

	"github.com/nareix/joy4/format"
)

const (
	idle = iota
	//	proceeding // recv invite, send 100 trying
	completed // send 200 OK
	confirmed // recv ACK
)

type Leg struct {
	callID  string
	fromTag string
	toTag   string
}
type sdpRemoteInfo struct {
	ssrc  int
	ip    string
	port  int
	proto string
	lPort int
	lip   string
}
type Invite struct {
	cfg    *config.Config
	state  int32
	leg    *Leg
	remote *sdpRemoteInfo
	rtp    *packet.RtpTransfer
	byed   chan bool
	sdp    *sdp.SDP
}

func init() {
	format.RegisterAll()
}
func NewInvite(cfg *config.Config) *Invite {
	rand.Seed(time.Now().UnixNano())
	return &Invite{cfg: cfg, state: idle, byed: make(chan bool)}
}

func (inv *Invite) HandleMsg(xlog *xlog.Logger, tr *transport.Transport, m *sip.Msg) {
	if m.CSeqMethod == sip.MethodInvite && strings.ToUpper(m.Payload.ContentType()) == "APPLICATION/SDP" {
		log.Println("recv invite msg")
		inv.InviteMsg(xlog, tr, m)
		return
	}
	if m.CSeqMethod == sip.MethodAck {
		inv.AckMsg(xlog, tr, m)
		return

		// send rtp msg
	}
	if m.CSeqMethod == sip.MethodBye {
		inv.ByeMsg(xlog, tr, m)
		return
	}

	xlog.Info("recv msg at ", inv.state, m)
}
func (inv *Invite) InviteMsg(xlog *xlog.Logger, tr *transport.Transport, m *sip.Msg) {
	// only handle invite idle state
	if atomic.LoadInt32(&inv.state) != idle {
		return
	}
	sdp, err := sdp.Parse(string(m.Payload.Data()))
	if err != nil {
		xlog.Error("parse sdp failed, err = ", err)
	}
	laHost := tr.Conn.LocalAddr().(*net.UDPAddr).IP.String()
	laPort := tr.Conn.LocalAddr().(*net.UDPAddr).Port
	r := &sdpRemoteInfo{
		ssrc: ssrc(sdp),
		ip:   sdp.Addr,
		//port:  int(sdp.Video.Port),
		lPort: randomFromStartEnd(10000, 65535),
		lip:   laHost,
	}
	proto := ""
	if sdp.Session == "Talk" {
		r.port = int(sdp.Audio.Port)
		proto = sdp.Audio.Proto
	} else {
		r.port = int(sdp.Video.Port)
		proto = sdp.Video.Proto
	}
	if strings.HasPrefix(proto, "TCP") {
		r.proto = "TCP"
	} else {
		r.proto = "UDP"
	}
	xlog.Info("[S->C] invite ", r.proto, "ssrc:", r.ssrc, "callId:", m.CallID)

	inv.remote = r
	inv.sdp = sdp

	resp := inv.makeRespFromReq(laHost, laPort, m, true, 200)
	inv.leg = &Leg{m.CallID, m.From.Param.Get("tag").Value, resp.To.Param.Get("tag").Value}
	atomic.StoreInt32(&inv.state, completed)
	xlog.Info("[C->S] 200OK(Invite)")
	tr.Send <- resp
}
func (inv *Invite) makeRespFromReq(localHost string, localPort int, req *sip.Msg, invite bool, code int) *sip.Msg {
	resp := &sip.Msg{
		Status:     code,
		From:       req.From.Copy(),
		To:         req.To.Copy(),
		CallID:     req.CallID,
		CSeq:       req.CSeq,
		CSeqMethod: req.CSeqMethod,
		UserAgent:  version.Version(),
		Via: &sip.Via{
			Version:  "2.0",
			Protocol: "SIP",
			Host:     localHost,
			Port:     uint16(localPort),
			Param:    &sip.Param{Name: "branch", Value: req.Via.Param.Get("branch").Value},
		},
	}

	if invite && code == 200 {
		resp.To.Tag()
		sdp := &sdp.SDP{
			Origin:  sdp.Origin{User: inv.cfg.GBID, Addr: localHost},
			Session: "play",
			Addr:    localHost,
			Video: &sdp.Media{
				//Proto:  inv.remote.proto + "/RTP/AVP",
				Proto: "TCP/RTP/AVP",

				Codecs: []sdp.Codec{{PT: uint8(96), Rate: 90000, Name: "PS"}},
				Port:   uint16(inv.remote.lPort)},
			SendOnly: true,
			Other:    [][2]string{{"y", strconv.Itoa(inv.remote.ssrc)}},
		}
		resp.Payload = sdp
	} else {
		toTag := util.GenerateTag()
		if inv.leg != nil {
			toTag = inv.leg.toTag
		}
		resp.To.Param = &sip.Param{Name: "tag", Value: toTag}
	}
	return resp
}
func ssrc(sdp *sdp.SDP) int {
	for _, i := range sdp.Other {
		if i[0] == "y" {
			ssrc, _ := strconv.ParseInt(i[1], 10, 64)
			return int(ssrc)
		}
	}
	return 0
}

func (inv *Invite) AckMsg(xlog *xlog.Logger, tr *transport.Transport, m *sip.Msg) {
	// only handle invite idle state
	if atomic.LoadInt32(&inv.state) != completed ||
		inv.leg.callID != m.CallID ||
		!strings.EqualFold(inv.leg.fromTag, m.From.Param.Get("tag").Value) {
		return
	}
	xlog.Info("[S->C] invite ack")
	atomic.StoreInt32(&inv.state, confirmed)
	// start send rtp
	if inv.sdp.Session == "Talk" {
		log.Println("invite talk")
		go inv.sendTalkRTPPacket(xlog)
	} else {
		inv.sendRTPPacket(xlog)
	}
}

func randomFromStartEnd(min, max int) int {

	return rand.Intn(max-min+1) + min
}
func (inv *Invite) sendRTPPacket(xlog *xlog.Logger) {
	//time.Sleep(10 * time.Second)
	//if inv.rtp != nil {
	//xlog.Info("rtp routine already exist, exit")
	//return
	//}
	var rtp *packet.RtpTransfer
	if inv.remote.proto == "UDP" {
		log.Println("new rtp transfer over udp, ip:", inv.remote.ip, "port:", inv.remote.port, "ssrc:", inv.remote.ssrc)
		rtp = packet.NewRRtpTransfer("", packet.UDPTransfer, inv.remote.ssrc)
	} else {
		log.Println("new rtp transfer over tcp, ssrc:", inv.remote.ssrc, "callid:", inv.leg.callID)
		//rtp = packet.NewRRtpTransfer("", packet.UDPTransfer, inv.remote.ssrc)
		rtp = packet.NewRRtpTransfer("", packet.TCPTransferActive, inv.remote.ssrc)
	}
	inv.rtp = rtp
	// send ip,port and recv ip,port
	err := inv.rtp.Service(inv.remote.lip, inv.remote.ip, inv.remote.lPort, inv.remote.port)
	if err != nil {
		xlog.Info("connect failed, err = ", err)
	}
	f, err := os.Open("test.dat")
	if err != nil {
		xlog.Errorf("read file error(%v)", err)
		rtp.Exit()
		return
	}

	defer func() {
		log.Println("exit send rtp pkt routine callid:", inv.leg.callID, "ssrc:", inv.remote.ssrc)
		f.Close()
		rtp.Exit()
		inv.rtp = nil
	}()

	buf, _ := ioutil.ReadAll(f)
	for {
		select {
		case <-inv.byed:
			log.Println("got signal inv.byed exit")
			goto end
		default:
			inv.sendFile(buf, rtp)
		}
	}
end:
}

func (inv *Invite) sendTalkRTPPacket(xlog *xlog.Logger) {
	log.Println("new rtp talk transfer over tcp, ssrc:", inv.remote.ssrc, "callid:", inv.leg.callID)
	rtp := packet.NewRRtpTransfer("", packet.TCPTransferActive, inv.remote.ssrc)
	err := rtp.Service(inv.remote.lip, inv.remote.ip, inv.remote.lPort, inv.remote.port)
	if err != nil {
		xlog.Info("connect failed, err = ", err)
		return
	}
	log.Println("start send rtp talk pkt")
	rtp.SendTalkRtp()
}

var i = 0
var pts uint64 = 0
var last = 0

func (inv *Invite) sendFile(buf []byte, rtp *packet.RtpTransfer) {
	if isPsHead(buf[i : i+4]) {
		stop := rtp.SendPSdata(buf[last:i], false, pts)
		if stop {
			return
		}
		pts += 40
		time.Sleep(time.Millisecond * 40)
		last = i
	}
	i++
	if i == len(buf) {
		log.Println("reset i to 0")
		i = 0
	}
}
func isPsHead(buf []byte) bool {
	h := []byte{0, 0, 1, 186}
	if len(buf) == 4 {
		for i := 0; i < 4; i++ {
			if buf[i] != h[i] {
				return false
			}
		}
		return true
	}
	return false
}

func (inv *Invite) ByeMsg(xlog *xlog.Logger, tr *transport.Transport, m *sip.Msg) {
	// only handle invite idle state
	if m.IsResponse() {
		return
	}
	xlog.Info("[S->C] bye, callId:", m.CallID)
	laHost := tr.Conn.LocalAddr().(*net.UDPAddr).IP.String()
	laPort := tr.Conn.LocalAddr().(*net.UDPAddr).Port
	xlog.Info("inv.state:", inv.state, "callId:", m.CallID)
	if atomic.LoadInt32(&inv.state) != confirmed ||
		inv.leg.callID != m.CallID ||
		!strings.EqualFold(inv.leg.fromTag, m.From.Param.Get("tag").Value) {
		resp := inv.makeRespFromReq(laHost, laPort, m, false, 481)
		xlog.Info("[C->S] 481(Bye)")
		tr.Send <- resp
		atomic.StoreInt32(&inv.state, idle)
		return
	}
	resp := inv.makeRespFromReq(laHost, laPort, m, false, 200)
	atomic.StoreInt32(&inv.state, idle)
	xlog.Info("[C->S] 200OK(Bye)")
	tr.Send <- resp
	xlog.Info("notify inv.byed")
	timeout := time.NewTimer(time.Millisecond * 500)
	select {
	case inv.byed <- true:
		break
	case <-timeout.C:
		xlog.Info("notify inv.byed timeout")
		break
	}
	xlog.Info("notify inv.byed done")
}
