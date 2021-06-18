package catalog

import (
	"bytes"
	"encoding/xml"
	"log"
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
	catalogInfo := catalog.sendCatalogResp(xlog, laHost, laPort, q.SN)
	go func() {
		log.Println("send catalog response")
		tr.Send <- catalogInfo
	}()
}

func (catalog *Catalog) sendCatalogResp(xlog *xlog.Logger, localHost string, localPort int, sn string) *sip.Msg {

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
	req.Payload = &catalogInfo{
		CmdType:  "Catalog",
		SN:       sn,
		DeviceID: catalog.cfg.GBID,
		SumNum:   strconv.Itoa(len(catalog.cfg.Devices)),
		DeviceList: DeviceList{
			Item: catalog.cfg.Devices,
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
	//data, _ := xml.MarshalIndent(cataInfo, " ", "    ")
	//bytes.Replace(data, []byte(""), []byte("\r\n"), -1)
	catalogBody := "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\r\n" +
		"<Response>\r\n" +
		"<CmdType>Catalog</CmdType>\r\n" +
		"<SN>0</SN>\r\n" +
		"<DeviceID>31011500991180000130</DeviceID>\r\n" +
		"<SumNum>4</SumNum>\r\n" +
		"<DeviceList Num=\"2\">\r\n" +
		"<Item>\r\n" +
		"<DeviceID>32011500991320000040</DeviceID>\r\n" +
		"<Name>test001</Name>\r\n" +
		"<Manufacturer>Hikvision</Manufacturer>\r\n" +
		"<Model>Mars</Model>\r\n" +
		"<Owner></Owner>\r\n" +
		"<CivilCode>civilCode</CivilCode>\r\n" +
		"<Address>192.18.1.1</Address>\r\n" +
		"<Parental>0</Parental>\r\n" +
		"<SafetyWay>1</SafetyWay>\r\n" +
		"<RegisterWay>1</RegisterWay>\r\n" +
		"<Secrecy>1</Secrecy>\r\n" +
		"<Status>ON</Status>\r\n" +
		"</Item>\r\n" +
		"<Item>\r\n" +
		"<DeviceID>32011500991320000050</DeviceID>\r\n" +
		"<Name>test002</Name>\r\n" +
		"<Manufacturer>Hikvision</Manufacturer>\r\n" +
		"<Model>Mars</Model>\r\n" +
		"<Owner></Owner>\r\n" +
		"<CivilCode>civilCode</CivilCode>\r\n" +
		"<Address>192.18.1.2</Address>\r\n" +
		"<Parental>0</Parental>\r\n" +
		"<SafetyWay>1</SafetyWay>\r\n" +
		"<RegisterWay>1</RegisterWay>\r\n" +
		"<Secrecy>1</Secrecy>\r\n" +
		"<Status>ON</Status>\r\n" +
		"</Item>\r\n" +
		"</DeviceList>\r\n" +
		"</Response>\r\n"
	return []byte(xml.Header + catalogBody)
}
