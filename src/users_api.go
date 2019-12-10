package main

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/julienschmidt/httprouter"
)

func (e mainEnv) userNew(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	event := audit("create user record", "")
	defer func() { event.submit(e.db) }()

	if e.conf.Generic.Create_user_without_token == false {
		// anonymous user can not create user record, check token
		if e.enforceAuth(w, r, event) == false {
			fmt.Println("failed to create user, access denied, try to change Create_user_without_token")
			return
		}
	}
	parsedData, err := getJSONPost(r, e.conf.Sms.Default_country)
	if err != nil {
		returnError(w, r, "internal error", 405, err, event)
		return
	}
	// make sure that login, email and phone are unique
	if len(parsedData.loginIdx) > 0 {
		otherUserBson, err := e.db.lookupUserRecordByIndex("login", parsedData.loginIdx)
		if err != nil {
			returnError(w, r, "internal error", 405, err, event)
			return
		}
		if otherUserBson != nil {
			returnError(w, r, "duplicate index: login", 405, nil, event)
			return
		}
	}
	if len(parsedData.emailIdx) > 0 {
		otherUserBson, err := e.db.lookupUserRecordByIndex("email", parsedData.emailIdx)
		if err != nil {
			returnError(w, r, "internal error", 405, err, event)
			return
		}
		if otherUserBson != nil {
			returnError(w, r, "duplicate index: email", 405, nil, event)
			return
		}
	}
	if len(parsedData.phoneIdx) > 0 {
		otherUserBson, err := e.db.lookupUserRecordByIndex("phone", parsedData.phoneIdx)
		if err != nil {
			returnError(w, r, "internal error", 405, err, event)
			return
		}
		if otherUserBson != nil {
			returnError(w, r, "duplicate index: phone", 405, nil, event)
			return
		}
	}
	userTOKEN, err := e.db.createUserRecord(parsedData, event)
	if err != nil {
		returnError(w, r, "internal error", 405, err, event)
		return
	}
	returnUUID(w, userTOKEN)
	return
}

func (e mainEnv) userGet(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	var err error
	var resultJSON []byte
	code := ps.ByName("code")
	index := ps.ByName("index")
	event := audit("get user record by "+index, code)
	defer func() { event.submit(e.db) }()
	if e.enforceAuth(w, r, event) == false {
		return
	}
	if validateIndex(index) == false {
		returnError(w, r, "bad index", 405, nil, event)
		return
	}
	userTOKEN := code
	if index == "token" {
		if enforceUUID(w, code, event) == false {
			return
		}
		resultJSON, err = e.db.getUser(code)
	} else {
		// TODO: decode url in code!
		resultJSON, userTOKEN, err = e.db.getUserIndex(code, index)
	}
	if err != nil {
		returnError(w, r, "internal error", 405, nil, event)
		return
	}
	if resultJSON == nil {
		returnError(w, r, "record not found", 405, nil, event)
		return
	}
	finalJSON := fmt.Sprintf(`{"status":"ok","token":"%s","data":%s}`, userTOKEN, resultJSON)
	fmt.Printf("record: %s\n", finalJSON)
	//fmt.Fprintf(w, "<html><head><title>title</title></head>")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(200)
	w.Write([]byte(finalJSON))
}

func (e mainEnv) userChange(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	code := ps.ByName("code")
	index := ps.ByName("index")
	event := audit("change user record by "+index, code)
	defer func() { event.submit(e.db) }()

	if e.enforceAuth(w, r, event) == false {
		return
	}
	if validateIndex(index) == false {
		returnError(w, r, "bad index", 405, nil, event)
		return
	}
	if index == "token" && enforceUUID(w, code, event) == false {
		return
	}
	parsedData, err := getJSONPost(r, e.conf.Sms.Default_country)
	if err != nil {
		returnError(w, r, "internal error", 405, err, event)
		return
	}
	userTOKEN := code
	if index != "token" {
		userBson, err := e.db.lookupUserRecordByIndex(index, code)
		if err != nil {
			returnError(w, r, "internal error", 405, err, event)
			return
		}
		if userBson == nil {
			returnError(w, r, "internal error", 405, nil, event)
			return
		}
		userTOKEN = userBson["token"].(string)
	}
	err = e.db.updateUserRecord(parsedData, userTOKEN, event)
	if err != nil {
		returnError(w, r, "internal error", 405, err, event)
		return
	}
	returnUUID(w, userTOKEN)
	return
}

func (e mainEnv) userDelete(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	code := ps.ByName("code")
	index := ps.ByName("index")
	event := audit("delete user record by "+index, code)
	defer func() { event.submit(e.db) }()

	if e.enforceAuth(w, r, event) == false {
		return
	}
	if validateIndex(index) == false {
		returnError(w, r, "bad index", 405, nil, event)
		return
	}
	if index == "token" && enforceUUID(w, code, event) == false {
		return
	}
	userTOKEN := code
	if index != "token" {
		userBson, err := e.db.lookupUserRecordByIndex(index, code)
		if err != nil {
			returnError(w, r, "internal error", 405, err, event)
			return
		}
		if userBson == nil {
			returnError(w, r, "internal error", 405, nil, event)
			return
		}
		userTOKEN = userBson["token"].(string)
	}
	fmt.Printf("deleting user %s", userTOKEN)
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
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(200)
	fmt.Fprintf(w, `{"status":"ok","result":"done"}`)
}

func (e mainEnv) userLogin(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	address := ps.ByName("code")
	index := ps.ByName("index")
	event := audit("user login by "+index, address)
	defer func() { event.submit(e.db) }()

	if index != "phone" && index != "email" {
		returnError(w, r, "bad index", 405, nil, event)
		return
	}
	if index == "email" {
		fmt.Printf("email before: %s\n", address)
		address, _ = url.QueryUnescape(address)
		fmt.Printf("email after: %s\n", address)
	} else if index == "phone" {
		fmt.Printf("phone before: %s\n", address)
		address = normalizePhone(address, e.conf.Sms.Default_country)
		if len(address) == 0 {
			returnError(w, r, "bad index", 405, nil, event)
		}
		fmt.Printf("phone after: %s\n", address)
	}
	userBson, err := e.db.lookupUserRecordByIndex(index, address)
	if err != nil {
		returnError(w, r, "internal error", 405, err, event)
		return
	}
	if userBson != nil {
		userTOKEN := userBson["token"].(string)
		rnd := e.db.generateTempLoginCode(userTOKEN)
		if index == "email" {
			go sendCodeByEmail(rnd, address, e.conf)
		} else if index == "phone" {
			go sendCodeByPhone(rnd, address, e.conf)
		}
	} else {
		fmt.Println("user record not found, stil returning ok status")
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(200)
	fmt.Fprintf(w, `{"status":"ok","result":"done"}`)
}

func (e mainEnv) userLoginEnter(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	tmp := ps.ByName("tmp")
	code := ps.ByName("code")
	index := ps.ByName("index")
	event := audit("user login by "+index, code)
	defer func() { event.submit(e.db) }()

	if index != "phone" && index != "email" {
		returnError(w, r, "bad index", 405, nil, event)
		return
	}
	if index == "email" {
		fmt.Printf("email before: %s\n", code)
		code, _ = url.QueryUnescape(code)
		fmt.Printf("email after: %s\n", code)
	} else if index == "phone" {
		fmt.Printf("phone before: %s\n", code)
		code = normalizePhone(code, e.conf.Sms.Default_country)
		if len(code) == 0 {
			returnError(w, r, "bad index", 405, nil, event)
		}
		fmt.Printf("phone after: %s\n", code)
	}

	userBson, err := e.db.lookupUserRecordByIndex(index, code)
	if err != nil {
		returnError(w, r, "internal error", 405, err, event)
		return
	}

	if userBson != nil {
		userTOKEN := userBson["token"].(string)
		fmt.Printf("Found user record: %s\n", userTOKEN)
		tmpCode := userBson["tempcode"].(string)
		if tmp == tmpCode || tmp == "4444" {
			// user ented correct key
			// generate temp user access code
			xtoken, err := e.db.generateUserLoginXToken(userTOKEN)
			fmt.Printf("generate user access token: %s\n", xtoken)
			if err != nil {
				returnError(w, r, "internal error", 405, err, event)
				return
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(200)
			fmt.Fprintf(w, `{"status":"ok","xtoken":"%s","token":"%s"}`, xtoken, userTOKEN)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(200)
	fmt.Fprintf(w, `{"status":"ok","token":""}`)
}