package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/golang/glog"

	"github.com/warik/gami"
	"github.com/warik/go-dialer/ami"
	"github.com/warik/go-dialer/conf"
	"github.com/warik/go-dialer/db"
	"github.com/warik/go-dialer/model"
	"github.com/warik/go-dialer/util"
)

type ApiHandler struct {
	p  interface{}
	fn func(p interface{}, w http.ResponseWriter, r *http.Request) (model.Response, error)
}

func (ah ApiHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var resp model.Response
	var err error

	err = withStructParams(ah.p, r)
	if err != nil {
		glog.Errorln(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if resp, err = ah.fn(ah.p, w, r); err != nil {
		glog.Errorln(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	} else {
		glog.Info("<<< PORTAL RESPONSE", resp)
		fmt.Fprint(w, resp)
	}
}

type AmiHandler struct {
	p  interface{}
	fn func(p interface{}, w http.ResponseWriter, r *http.Request) (gami.Message, error, string)
}

func (ah AmiHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if ah.p != nil {
		var err error
		if *signedInput {
			err = withSignedParams(ah.p, r)
		} else {
			err = withStructParams(ah.p, r)
		}
		if err != nil {
			glog.Errorln(err)
			fmt.Fprint(w, model.Response{"status": "error", "error": err.Error()})
			return
		}
	}

	resp, err, dataKey := ah.fn(ah.p, w, r)
	if err != nil {
		glog.Errorln(err)
		fmt.Fprint(w, model.Response{"status": "error", "error": err.Error()})
		return
	}
	if r, ok := resp["Response"]; ok && r == "Follows" {
		glog.Infoln("<<< RESPONSE...")
	} else {
		glog.Infoln("<<< RESPONSE", resp)
	}

	status := strings.ToLower(resp["Response"])

	response := ""
	if val, ok := resp[dataKey]; ok {
		response = val
		status = "success"
	} else {
		status = "error"
	}
	fmt.Fprint(w, model.Response{"status": status, "response": response})
}

func withSignedParams(i interface{}, r *http.Request) error {
	signedData := new(model.SignedInputData)
	if err := model.GetStructFromParams(r, signedData); err != nil {
		return err
	}
	glog.Infoln("<<< SIGNED PARAMS", signedData)
	if err := util.UnsignData(i, (*signedData)); err != nil {
		return err
	}
	glog.Infoln("<<< INPUT PARAMS", i)
	return nil
}

func withStructParams(i interface{}, r *http.Request) error {
	if err := model.GetStructFromParams(r, i); err != nil {
		return err
	}
	glog.Infoln("<<< INPUT PARAMS", i)
	return nil
}

func Stats(w http.ResponseWriter, r *http.Request) {
	page := `
		<h1>{{.Name}} dialer stats</h1>
		<b>DB CDR</b>: {{.DBCount}}
	`
	t, _ := template.New("stats").Parse(page)
	stats := model.DialerStats{conf.GetConf().Name, db.GetDB().GetCdrCount()}
	t.Execute(w, stats)
}

func CdrCount(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, model.Response{"number_of_cdrs": strconv.Itoa(db.GetDB().GetCdrCount())})
}

func DeleteCdr(p interface{}, w http.ResponseWriter, r *http.Request) (model.Response, error) {
	cdr := (*p.(*model.Cdr))
	res, err := db.GetDB().DeleteCdr(cdr.Id)
	if err != nil {
		return nil, err
	} else if count, _ := res.RowsAffected(); count != 1 {
		return model.Response{"status": "error", "message": "result is not 1"}, nil
	} else {
		return model.Response{"status": "success"}, nil
	}
}

func GetCdr(p interface{}, w http.ResponseWriter, r *http.Request) (model.Response, error) {
	cdr := (*p.(*model.Cdr))
	returnCdr, err := db.GetDB().GetCDR(cdr.UniqueID)
	if err != nil {
		return nil, err
	} else {
		return model.Response{"cdr": fmt.Sprintf("%#v", returnCdr)}, nil
	}
}

func ManagerCallAfterHours(p interface{}, w http.ResponseWriter,
	r *http.Request) (model.Response, error) {
	phoneCall := (*p.(*model.PhoneCall))
	settings := conf.GetConf().Agencies[phoneCall.Country]
	payload, _ := json.Marshal(model.Dict{
		"calling_phone": phoneCall.CallingPhone,
		"CompanyId":     settings.CompanyId,
	})
	url := conf.GetConf().GetApi(phoneCall.Country, "manager_call_after_hours")
	resp, err := util.SendRequest(payload, url, "POST", settings.Secret, settings.CompanyId)
	return model.Response{"status": resp}, err
}

func ShowCallingReview(p interface{}, w http.ResponseWriter, r *http.Request) (model.Response, error) {
	phoneCall := (*p.(*model.PhoneCall))
	resp, err := util.ShowReviewPopup(
		phoneCall.ReviewHref,
		phoneCall.InnerNumber,
		phoneCall.Country,
	)
	return model.Response{"status": resp}, err
}

func ShowCallingPopup(p interface{}, w http.ResponseWriter, r *http.Request) (model.Response, error) {
	phoneCall := (*p.(*model.PhoneCall))
	resp, err := util.ShowCallingPopup(
		phoneCall.InnerNumber,
		phoneCall.CallingPhone,
		phoneCall.Country,
	)
	return model.Response{"status": resp}, err
}

func ManagerPhoneForCompany(p interface{}, w http.ResponseWriter, r *http.Request) (model.Response, error) {
	phoneCall := (*p.(*model.PhoneCall))
	settings := conf.GetConf().Agencies[phoneCall.Country]
	payload, _ := json.Marshal(model.Dict{
		"id":        phoneCall.Id,
		"CompanyId": settings.CompanyId,
	})
	url := conf.GetConf().GetApi(phoneCall.Country, "manager_phone_for_company")
	resp, err := util.SendRequest(payload, url, "GET", settings.Secret, settings.CompanyId)
	return model.Response{"inner_number": resp}, err
}

func ManagerPhone(p interface{}, w http.ResponseWriter, r *http.Request) (model.Response, error) {
	phoneCall := (*p.(*model.PhoneCall))
	settings := conf.GetConf().Agencies[phoneCall.Country]
	payload, _ := json.Marshal(model.Dict{
		"calling_phone": phoneCall.CallingPhone,
		"CompanyId":     settings.CompanyId,
	})
	url := conf.GetConf().GetApi(phoneCall.Country, "manager_phone")
	resp, err := util.SendRequest(payload, url, "GET", settings.Secret, settings.CompanyId)
	return model.Response{"inner_number": resp}, err
}

func QueueRemove(p interface{}, w http.ResponseWriter, r *http.Request) (gami.Message, error, string) {
	qc := (*p.(*model.QueueContainer))
	resp, err := ami.RemoveFromQueue(qc.Queue, qc.InnerNumber)
	return resp, err, "Message"
}

func QueueAdd(p interface{}, w http.ResponseWriter, r *http.Request) (gami.Message, error, string) {
	qc := (*p.(*model.QueueContainer))
	resp, err := ami.AddToQueue(qc.Queue, qc.InnerNumber)
	if err != nil {
		return resp, err, ""
	}
	resp, err = ami.QueueStatus(qc.Queue, qc.InnerNumber)
	if err != nil {
		return resp, err, ""
	}
	return resp, err, "StatusKey"
}

func QueueStatus(p interface{}, w http.ResponseWriter, r *http.Request) (gami.Message, error, string) {
	qc := (*p.(*model.QueueContainer))
	resp, err := ami.QueueStatus(qc.Queue, qc.InnerNumber)
	return resp, err, "StatusKey"
}

func PlaceSpy(p interface{}, w http.ResponseWriter, r *http.Request) (gami.Message, error, string) {
	call := (*p.(*model.Call))
	resp, err := ami.Spy(call)
	return resp, err, "Message"
}

func ShowInuse(p interface{}, w http.ResponseWriter, r *http.Request) (gami.Message, error, string) {
	resp, err := ami.GetActiveChannels()
	return resp, err, "CmdData"
}

func PlaceCall(p interface{}, w http.ResponseWriter, r *http.Request) (gami.Message, error, string) {
	resp, err := ami.Call(*p.(*model.Call))
	return resp, err, "Message"
}

func PlaceCallInQueue(p interface{}, w http.ResponseWriter, r *http.Request) (gami.Message, error, string) {
	resp, err := ami.CallInQueue(*p.(*model.CallInQueue))
	return resp, err, "Message"
}

func PingAsterisk(p interface{}, w http.ResponseWriter, r *http.Request) (gami.Message, error, string) {
	resp, err := ami.Ping()
	return resp, err, "Ping"
}

func CheckPortals(w http.ResponseWriter, r *http.Request) {
	for country, settings := range conf.GetConf().Agencies {
		url := conf.GetConf().GetApi(country, "manager_phone")
		payload, _ := json.Marshal(model.Dict{
			"calling_phone": "6916",
			"CompanyId":     settings.CompanyId,
		})
		if _, err := util.SendRequest(payload, url, "GET", settings.Secret,
			settings.CompanyId); err != nil {
			fmt.Fprint(w, model.Response{"country": country, "error": err.Error()})
		} else {
			fmt.Fprint(w, model.Response{"country": country, "ok": "ok"})
		}
	}
}

func ImUp(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "Im up, Im up...")
}
