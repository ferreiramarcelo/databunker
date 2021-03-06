package main

import (
	"fmt"
	"net/http"

	"github.com/julienschmidt/httprouter"
)

func (e mainEnv) getUserRequests(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	if e.enforceAuth(w, r, nil) == "" {
		return
	}
	var offset int32
	var limit int32 = 10
	status := "open"
	args := r.URL.Query()
	if value, ok := args["offset"]; ok {
		offset = atoi(value[0])
	}
	if value, ok := args["limit"]; ok {
		limit = atoi(value[0])
	}
	if value, ok := args["status"]; ok {
		status = value[0]
	}
	resultJSON, counter, err := e.db.getRequests(status, offset, limit)
	if err != nil {
		returnError(w, r, "internal error", 405, err, nil)
		return
	}
	fmt.Printf("Total count of user requests: %d\n", counter)
	//fmt.Fprintf(w, "<html><head><title>title</title></head>")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(200)
	str := fmt.Sprintf(`{"status":"ok","total":%d,"rows":%s}`, counter, resultJSON)
	w.Write([]byte(str))
}

func (e mainEnv) getUserRequest(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	request := ps.ByName("request")
	event := audit("get user request by request token", request, "request", request)
	defer func() { event.submit(e.db) }()

	if enforceUUID(w, request, event) == false {
		return
	}
	authResult := e.enforceAuth(w, r, event)
	if authResult == "" {
		return
	}
	requestInfo, err := e.db.getRequest(request)
	if err != nil {
		fmt.Printf("%d access denied for: %s\n", http.StatusForbidden, request)
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("Access denied"))
		return
	}
	if len(requestInfo) == 0 {
		returnError(w, r, "not found", 405, err, event)
		return
	}
	var resultJSON []byte
	userTOKEN := ""
	appName := ""
	change := ""
	if value, ok := requestInfo["token"]; ok {
		userTOKEN = value.(string)
		event.Record = userTOKEN
	}
	if value, ok := requestInfo["change"]; ok {
		change = value.(string)
	}
	if value, ok := requestInfo["app"]; ok {
		appName = value.(string)
	}
	if len(appName) > 0 {
		resultJSON, err = e.db.getUserApp(userTOKEN, appName)
	} else {
		resultJSON, err = e.db.getUser(userTOKEN)
	}
	if err != nil {
		returnError(w, r, "internal error", 405, err, event)
		return
	}
	if resultJSON == nil {
		returnError(w, r, "not found", 405, err, event)
		return
	}
	//fmt.Printf("Full json: %s\n", resultJSON)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(200)
	var str string

	if len(appName) > 0 {
		str = fmt.Sprintf(`"status":"ok","app":"%s"`, appName)
	} else {
		str = fmt.Sprintf(`"status":"ok"`)
	}
	if len(resultJSON) > 0 {
		str = fmt.Sprintf(`%s,"original":%s`, str, resultJSON)
	}
	if len(change) > 0 {
		str = fmt.Sprintf(`%s,"change":%s`, str, change)
	}
	str = fmt.Sprintf(`{%s}`, str)
	//fmt.Printf("result: %s\n", str)
	w.Write([]byte(str))
}

func (e mainEnv) approveUserRequest(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	request := ps.ByName("request")
	event := audit("approve user request", request, "request", request)
	defer func() { event.submit(e.db) }()

	if enforceUUID(w, request, event) == false {
		return
	}
	authResult := e.enforceAuth(w, r, event)
	if authResult == "" {
		return
	}
	requestInfo, err := e.db.getRequest(request)
	if err != nil {
		fmt.Printf("%d access denied for: %s\n", http.StatusForbidden, request)
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("Access denied"))
		return
	}
	if len(requestInfo) == 0 {
		returnError(w, r, "not found", 405, err, event)
		return
	}
	userTOKEN := ""
	action := ""
	if value, ok := requestInfo["action"]; ok {
		action = value.(string)
	}
	if value, ok := requestInfo["token"]; ok {
		userTOKEN = value.(string)
		event.Record = userTOKEN
	}
	resultJSON, err := e.db.getUser(userTOKEN)
	if err != nil {
		returnError(w, r, "internal error", 405, err, event)
		return
	}
	if resultJSON == nil {
		returnError(w, r, "not found", 405, err, event)
		return
	}
	if action == "forget-me" {
		result, err := e.db.deleteUserRecord(userTOKEN)
		if err != nil {
			returnError(w, r, "internal error", 405, err, event)
			return
		}
		if result == false {
			// user deleted
			event.Status = "failed"
			event.Msg = "failed to delete"
		}
		notifyURL := e.conf.Notification.ForgetmeNotificationURL
		notifyForgetMe(notifyURL, resultJSON, "token", userTOKEN)
		e.db.updateRequestStatus(request, "approve")
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(200)
	fmt.Fprintf(w, `{"status":"ok","result":"done"}`)
}

func (e mainEnv) cancelUserRequest(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	request := ps.ByName("request")
	event := audit("cancel user request", request, "request", request)
	defer func() { event.submit(e.db) }()

	if enforceUUID(w, request, event) == false {
		return
	}
	authResult := e.enforceAuth(w, r, event)
	if authResult == "" {
		return
	}
	requestInfo, err := e.db.getRequest(request)
	if err != nil {
		fmt.Printf("%d access denied for: %s\n", http.StatusForbidden, request)
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("Access denied"))
		return
	}
	if len(requestInfo) == 0 {
		returnError(w, r, "not found", 405, err, event)
		return
	}
	userTOKEN := ""
	if value, ok := requestInfo["token"]; ok {
		userTOKEN = value.(string)
		event.Record = userTOKEN
	}
	if requestInfo["status"].(string) != "open" {
		returnError(w, r, "wrong status", 405, err, event)
		return
	}
	resultJSON, err := e.db.getUser(userTOKEN)
	if err != nil {
		returnError(w, r, "internal error", 405, err, event)
		return
	}
	if resultJSON == nil {
		returnError(w, r, "not found", 405, err, event)
		return
	}
	e.db.updateRequestStatus(request, "cancel")

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(200)
	fmt.Fprintf(w, `{"status":"ok","result":"done"}`)
}
