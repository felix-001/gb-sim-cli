package catalog

import (
	"bytes"
	"encoding/xml"
	"net"
	"strconv"
	"time"

	"github.com/jart/gosip/sip"
	"github.com/jart/gosip/util"
	"github.com/lzh2nix/gb28181Simulator/internal/config"
	"github.com/lzh2nix/gb28181Simulator/internal/transport"
	"github.com/lzh2nix/gb28181Simulator/internal/version"
	"github.com/qiniu/x/xlog"
	"golang.org/x/net/html/charset"
)

type Catalog struct {
	cfg *config.Config
}

func NewCatalog(cfg *config.Config) *Catalog {

	return &Catalog{cfg: cfg}
}

type catalogQuery struct {
	XMLName  xml.Name `xml:"Query"`
	Text     string   `xml:",chardata"`
	CmdType  string   `xml:"CmdType"`
	SN       string   `xml:"SN"`
	DeviceID string   `xml:"DeviceID"`
}

func (catalog *Catalog) Handle(xlog *xlog.Logger, tr *transport.Transport, req *sip.Msg) {

	// 1.send 200 ok response
	laHost := tr.Conn.LocalAddr().(*net.UDPAddr).IP.String()
	laPort := tr.Conn.LocalAddr().(*net.UDPAddr).Port
	resp := catalog.makeCatalogRespFromReq(laHost, laPort, req)
	tr.Send <- resp
	time.Sleep(time.Millisecond * 10)
	// 2. send catalog msg
	var q catalogQuery
	decoder := xml.NewDecoder(bytes.NewReader([]byte(req.Payload.Data())))
	decoder.CharsetReader = charset.NewReaderLabel
	if err := decoder.Decode(&q); err != nil {
		xlog.Errorf("unmarsh xml failed, err = %#v", err, "msg  = ", req)
		return
	}
	if err := xml.Unmarshal(req.Payload.Data(), &q); err != nil {

	}
	catalogInfo := catalog.sendCatalogResp(xlog, laHost, laPort, q.SN, "", "")
	//	log.Printf("catalogInfo0 addr: %p\n", catalogInfo)
	go func() {
		//log.Println("send catalog response1")
		tr.Send <- catalogInfo
		//log.Println("ch len after send catalog1:", len(tr.Send))
	}()
	catalogInfo1 := catalog.sendCatalogResp(xlog, laHost, laPort, q.SN, "32011500991320000081", "AlarmIn")
	//log.Printf("catalogInfo1 addr: %p\n", catalogInfo1)
	go func() {
		//log.Println("send catalog response2")
		tr.Send <- catalogInfo1
		//log.Println("ch len after send catalog2:", len(tr.Send))
	}()
	catalogInfo2 := catalog.sendCatalogResp(xlog, laHost, laPort, q.SN, "32011500991320000050", "")
	//log.Printf("catalogInfo2 addr: %p\n", catalogInfo2)
	go func() {
		//		log.Println("send catalog response3")
		tr.Send <- catalogInfo2
		//	log.Println("ch len after send catalog3:", len(tr.Send))
	}()
	catalogInfo3 := catalog.sendCatalogResp(xlog, laHost, laPort, q.SN, "32011500991320000051", "AlarmIn")
	//log.Printf("catalogInfo3 addr: %p\n", catalogInfo3)
	go func() {
		//log.Println("send catalog response4")
		tr.Send <- catalogInfo3
		//log.Println("ch len after send catalog4:", len(tr.Send))
	}()
	catalogInfo4 := catalog.sendCatalogResp(xlog, laHost, laPort, q.SN, "32011500991320000060", "")
	//log.Printf("catalogInfo4 addr: %p\n", catalogInfo4)
	go func() {
		//log.Println("send catalog response5")
		tr.Send <- catalogInfo4
		//log.Println("ch len after send catalog5:", len(tr.Send))
	}()
	catalogInfo5 := catalog.sendCatalogResp(xlog, laHost, laPort, q.SN, "32011500991320000061", "AlarmIn")
	//log.Printf("catalogInfo5 addr: %p\n", catalogInfo5)
	go func() {
		//log.Println("send catalog response6")
		tr.Send <- catalogInfo5
		//log.Println("ch len after send catalog6:", len(tr.Send))
	}()
}

func (catalog *Catalog) sendCatalogResp(xlog *xlog.Logger, localHost string, localPort int, sn, chid, model string) *sip.Msg {

	req := &sip.Msg{
		CSeq:       util.GenerateCSeq(),
		CallID:     util.GenerateCallID(),
		Method:     sip.MethodMessage,
		CSeqMethod: sip.MethodMessage,
		UserAgent:  version.Version(),
		Request: &sip.URI{
			Scheme: "sip",
			User:   catalog.cfg.ServerID,
			Host:   catalog.cfg.Realm,
		},
		Via: &sip.Via{
			Version:  "2.0",
			Protocol: "SIP",
			Host:     localHost,
			Port:     uint16(localPort),

			Param: &sip.Param{Name: "branch", Value: util.GenerateBranch()},
		},
		Contact: &sip.Addr{
			Uri: &sip.URI{
				User: catalog.cfg.GBID,
				Host: localHost,
				Port: uint16(localPort),
			},
		},
		From: &sip.Addr{
			Uri: &sip.URI{
				User: catalog.cfg.GBID,
				Host: catalog.cfg.Realm,
			},
			Param: &sip.Param{Name: "tag", Value: util.GenerateTag()},
		},
		To: &sip.Addr{
			Uri: &sip.URI{
				User: catalog.cfg.ServerID,
				Host: catalog.cfg.Realm,
			},
		},
	}

	devices := make([]config.DeviceInfo, 1)
	copy(devices, catalog.cfg.Devices)
	if model != "" {
		devices[0].Model = model
	}
	if chid != "" {
		devices[0].DeviceID = chid
	}
	req.Payload = &catalogInfo{
		CmdType:  "Catalog",
		SN:       sn,
		DeviceID: catalog.cfg.GBID,
		SumNum:   strconv.Itoa(len(catalog.cfg.Devices)),
		DeviceList: DeviceList{
			//Item: catalog.cfg.Devices,
			Item: devices,
			Num:  strconv.Itoa(len(catalog.cfg.Devices)),
		},
	}
	return req
}
func (catalog *Catalog) makeCatalogRespFromReq(localHost string, localPort int, req *sip.Msg) *sip.Msg {
	resp := sip.Msg{
		Status:     200,
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
	resp.To.Tag()
	return &resp
}

type DeviceList struct {
	Text string              `xml:",chardata"`
	Num  string              `xml:"Num,attr"`
	Item []config.DeviceInfo `xml:"Item"`
}
type catalogInfo struct {
	XMLName    xml.Name   `xml:"Response"`
	Text       string     `xml:",chardata"`
	CmdType    string     `xml:"CmdType"`
	SN         string     `xml:"SN"`
	DeviceID   string     `xml:"DeviceID"`
	SumNum     string     `xml:"SumNum"`
	DeviceList DeviceList `xml:"DeviceList"`
}

func (cataInfo *catalogInfo) ContentType() string {
	return "Application/MANSCDP+xml"
}
func (cataInfo *catalogInfo) Data() []byte {
	data, _ := xml.MarshalIndent(cataInfo, "  ", "    ")
	return []byte(xml.Header + string(data))
}
