package packet

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/qiniu/x/xlog"
)

var log *xlog.Logger

func init() {
	log = xlog.New("streams")
}

// RtpTransfer ...
type RtpTransfer struct {
	datasrc      string
	protocol     int // tcp or udp
	psEnc        *encPSPacket
	payload      chan []byte
	cseq         uint16
	ssrc         uint32
	udpconn      *net.UDPConn
	tcpconn      net.Conn
	writestop    chan bool
	quit         chan bool
	timerProcess *time.Ticker
	Stop         bool
}

func NewRRtpTransfer(src string, pro int, ssrc int) *RtpTransfer {

	return &RtpTransfer{
		datasrc:   src,
		protocol:  pro,
		psEnc:     &encPSPacket{},
		payload:   make(chan []byte, 25),
		cseq:      0,
		ssrc:      uint32(ssrc),
		writestop: make(chan bool, 1),
		quit:      make(chan bool, 1),
		Stop:      false,
	}
}

// Service ...
func (rtp *RtpTransfer) Service(srcip, dstip string, srcport, dstport int) error {

	if nil == rtp.timerProcess {
		rtp.timerProcess = time.NewTicker(time.Second * time.Duration(5))
	}
	if rtp.protocol == TCPTransferPassive {
		go rtp.write4tcppassive(srcip+":"+strconv.Itoa(srcport),
			dstip+":"+strconv.Itoa(dstport))

	} else if rtp.protocol == TCPTransferActive {
		// connect to to dst ip port
		go rtp.write4tcpactive(dstip, dstport)
	} else if rtp.protocol == UDPTransfer {
		conn, err := net.DialUDP("udp",
			&net.UDPAddr{
				IP:   net.ParseIP(srcip),
				Port: srcport,
			},
			&net.UDPAddr{
				IP:   net.ParseIP(dstip),
				Port: dstport,
			})
		if err != nil {
			return err
		}
		rtp.udpconn = conn
		go rtp.write4udp()
	} else if rtp.protocol == LocalCache {
		// write file
		go rtp.write4file()
	} else {
		return errors.New("unknown transfer way")
	}
	return nil
}

// Exit ...
func (rtp *RtpTransfer) Exit() {

	if nil != rtp.timerProcess {
		rtp.timerProcess.Stop()
	}
	close(rtp.writestop)
	<-rtp.quit
}

func (rtp *RtpTransfer) Send2data(data []byte, key bool, pts uint64) {
	psSys := rtp.psEnc.encPackHeader(pts)
	if key { // just I frame will add this
		psSys = rtp.psEnc.encSystemHeader(psSys, 2048, 512)
		psSys = rtp.psEnc.encProgramStreamMap(psSys)
	}

	lens := len(data)
	var index int
	for lens > 0 {
		pesload := lens
		if pesload > PESLoadLength {
			pesload = PESLoadLength
		}
		pes := rtp.psEnc.encPESPacket(data[index:index+pesload], StreamIDVideo, pesload, pts, pts)

		// every frame add ps header
		if index == 0 {
			pes = append(psSys, pes...)
		}
		// remain data to package again
		// over the max pes len and split more pes load slice
		index += pesload
		lens -= pesload
		if lens > 0 {
			rtp.fragmentation(pes, pts, 0)
		} else {
			// the last slice
			rtp.fragmentation(pes, pts, 1)

		}
	}

}

func (rtp *RtpTransfer) SendPSdata(data []byte, key bool, pts uint64) bool {
	lens := len(data)
	var index int
	for lens > 0 {
		pesload := lens
		if pesload > PESLoadLength {
			pesload = PESLoadLength
		}
		pes := data[index : index+pesload]

		// every frame add ps header
		// remain data to package again
		// over the max pes len and split more pes load slice
		index += pesload
		lens -= pesload
		stop := false
		if lens > 0 {
			stop = rtp.fragmentation(pes, pts, 0)
		} else {
			// the last slice
			stop = rtp.fragmentation(pes, pts, 1)
		}
		if stop {
			return stop
		}

	}
	return false
}

func (rtp *RtpTransfer) SendTalkRtp() {
	payload := rtp.encRtpHeader([]byte{1, 2, 3}, 1, 0)
	rtp.payload <- payload
}

func (rtp *RtpTransfer) fragmentation(data []byte, pts uint64, last int) bool {
	datalen := len(data)
	if datalen+RTPHeaderLength <= RtpLoadLength {
		//if rtp.Stop {
		//	return true
		//}
		payload := rtp.encRtpHeader(data[:], 1, pts)
		rtp.payload <- payload
	} else {
		marker := 0
		var index int
		sendlen := RtpLoadLength - RTPHeaderLength
		for datalen > 0 {
			if datalen <= sendlen {
				marker = 1
				sendlen = datalen
			}
			payload := rtp.encRtpHeader(data[index:index+sendlen], marker&last, pts)
			//if rtp.Stop {
			//	return true
			//}
			rtp.payload <- payload
			datalen -= sendlen
			index += sendlen
		}
	}
	return false
}
func (rtp *RtpTransfer) encRtpHeader(data []byte, marker int, curpts uint64) []byte {

	if rtp.protocol == LocalCache {
		return data
	}
	rtp.cseq++
	pack := make([]byte, RTPHeaderLength)
	bits := bitsInit(RTPHeaderLength, pack)
	bitsWrite(bits, 2, 2)
	bitsWrite(bits, 1, 0)
	bitsWrite(bits, 1, 0)
	bitsWrite(bits, 4, 0)
	bitsWrite(bits, 1, uint64(marker))
	bitsWrite(bits, 7, 96)
	bitsWrite(bits, 16, uint64(rtp.cseq))
	bitsWrite(bits, 32, curpts)
	bitsWrite(bits, 32, uint64(rtp.ssrc))
	if rtp.protocol != UDPTransfer {
		var rtpOvertcp []byte
		lens := len(data) + 12
		rtpOvertcp = append(rtpOvertcp, byte(uint16(lens)>>8), byte(uint16(lens)))
		rtpOvertcp = append(rtpOvertcp, bits.pData...)
		return append(rtpOvertcp, data...)
	}
	return append(bits.pData, data...)

}

func (rtp *RtpTransfer) write4udp() {

	log.Infof("write4udp stream data will be write by(udp)")
	for {
		select {
		case data, ok := <-rtp.payload:
			if ok {
				if rtp.udpconn != nil {
					lens, err := rtp.udpconn.Write(data)
					if err != nil || lens != len(data) {
						log.Errorf("write data by udp error(%v), len(%v).", err, lens)
						goto UDPSTOP
					}
				}
			} else {
				log.Error("rtp data channel closed")
				goto UDPSTOP
			}
		case <-rtp.timerProcess.C:
			log.Error("channel recv data timeout")
			goto UDPSTOP
		case <-rtp.writestop:
			log.Error("udp rtp send channel stop")
			goto UDPSTOP
		}
	}
UDPSTOP:
	rtp.udpconn.Close()
	rtp.Stop = true
	rtp.quit <- true
}

func (rtp *RtpTransfer) write4tcppassive(srcaddr, dstaddr string) {

	log.Infof(" stream data will be write by(%v)", rtp.protocol)
	addr, err := net.ResolveTCPAddr("tcp", srcaddr)
	if err != nil {
		log.Errorf("net.ResolveTCPAddr error(%v).", err)
		return
	}
	listener, err := net.ListenTCP("tcp", addr)
	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}
		rtp.tcpconn = conn
		break
	}
	for {
		if rtp.tcpconn == nil {
			goto TCPPASSIVESTOP
		}
		select {
		case data, ok := <-rtp.payload:
			if ok {
				lens, err := rtp.tcpconn.Write(data)
				if err != nil || lens != len(data) {
					log.Errorf("write data by tcp error(%v), len(%v).", err, lens)
					goto TCPPASSIVESTOP
				}

			} else {
				log.Errorf("data channel closed")
				goto TCPPASSIVESTOP
			}
		case <-rtp.timerProcess.C:
			log.Error("channel write data timeout when tcp send")
			goto TCPPASSIVESTOP
		case <-rtp.writestop:
			log.Error("tcp rtp send channel stop")
			goto TCPPASSIVESTOP
		}
	}
TCPPASSIVESTOP:
	rtp.tcpconn.Close()
	rtp.quit <- true
}

func (rtp *RtpTransfer) write4tcpactive(dstaddr string, port int) {

	log.Infof("write4tcpactive stream data will be write by(tcp)")
	var err error
	rtp.tcpconn, err = net.Dial("tcp", fmt.Sprintf("%s:%d", dstaddr, port))
	if err != nil {
		log.Fatalln(err)
	} else {
		log.Println("tcp connet to", dstaddr, ":", port, "success", rtp.tcpconn.LocalAddr().String())
	}
	defer func() {
		log.Println("write4tcpactive routine exit, ", rtp.tcpconn.LocalAddr().String())
		rtp.tcpconn.Close()
		rtp.quit <- true
	}()

	go func() {
		buf := make([]byte, 1024)
		for {
			_, err := rtp.tcpconn.Read(buf)
			if err != nil {
				log.Error("tcp read error", err, rtp.tcpconn.LocalAddr().String())
				rtp.writestop <- true
				return
			}
			log.Println("tcp recv rtp data", rtp.tcpconn.LocalAddr().String(), "len", len(buf), "data:", string(buf))
		}
	}()

	count := 0
	for {
		select {
		case data, ok := <-rtp.payload:
			if ok {
				lens, err := rtp.tcpconn.Write(data)
				if err != nil || lens != len(data) {
					log.Error("write data by tcp error", err, lens, len(data), rtp.tcpconn.LocalAddr().String())
					goto end
				}
				if count%6000 == 0 {
					log.Println("already send", count, "rtp pkts", rtp.tcpconn.LocalAddr().String())
				}
				count++

			} else {
				log.Errorf("data channel closed")
				break
			}
		case <-rtp.writestop:
			log.Error("tcp rtp send channel stop")
			goto end
		}
	}
end:
	rtp.Stop = true
}

func (rtp *RtpTransfer) write4file() {

	log.Infof(" stream data will be write by(%v)", rtp.protocol)
	files, err := os.OpenFile("./test.dat", os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Errorf("open test.dat file err(%v", err)
		return
	}

	for {
		select {
		case data, ok := <-rtp.payload:
			if ok {
				lens, err := files.Write(data)
				if err != nil || lens != len(data) {
					log.Errorf("write data by file error(%v), len(%v).", err, lens)
					goto FILESTOP
				}

			} else {
				log.Error("data channel closed when write file")
				goto FILESTOP
			}
		case <-rtp.writestop:
			log.Error("write file channel stop")
			goto FILESTOP
		}
	}
FILESTOP:
	files.Close()
	rtp.quit <- true
}
