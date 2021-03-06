package main

import (
	"fmt"
	"net/http"

	"github.com/julienschmidt/httprouter"
)

func (e mainEnv) userNew(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	event := audit("create user record", "", "", "")
	defer func() { event.submit(e.db) }()

	if e.conf.Generic.CreateUserWithoutAccessToken == false {
		// anonymous user can not create user record, check token
		if e.enforceAuth(w, r, event) == "" {
			fmt.Println("failed to create user, access denied, try to change Create_user_without_access_token")
			return
		}
	}
	parsedData, err := getJSONPost(r, e.conf.Sms.DefaultCountry)
	if err != nil {
		returnError(w, r, "failed to decode request body", 405, err, event)
		return
	}
	if len(parsedData.jsonData) == 0 {
		returnError(w, r, "empty body", 405, nil, event)
		return
	}
	// make sure that login, email and phone are unique
	if len(parsedData.loginIdx) > 0 {
		otherUserBson, err := e.db.lookupUserRecordByIndex("login", parsedData.loginIdx, e.conf)
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
		otherUserBson, err := e.db.lookupUserRecordByIndex("email", parsedData.emailIdx, e.conf)
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
		otherUserBson, err := e.db.lookupUserRecordByIndex("phone", parsedData.phoneIdx, e.conf)
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
	if len(parsedData.emailIdx) > 0 {
		e.db.linkConsentRecords(userTOKEN, "email", parsedData.emailIdx)
	}
	if len(parsedData.phoneIdx) > 0 {
		e.db.linkConsentRecords(userTOKEN, "phone", parsedData.phoneIdx)
	}
	if len(parsedData.emailIdx) > 0 && len(parsedData.phoneIdx) > 0 {
		// delete duplicate consent records for user
		records, _ := e.db.getList(TblName.Consent, "who", parsedData.emailIdx, 0, 0)
		var briefCodes []string
		for _, val := range records {
			fmt.Printf("adding brief code: %s\n", val["brief"].(string))
			briefCodes = append(briefCodes, val["brief"].(string))
		}
		records, _ = e.db.getList(TblName.Consent, "who", parsedData.phoneIdx, 0, 0)
		for _, val := range records {
			fmt.Printf("XXX checking brief code for duplicates: %s\n", val["brief"].(string))
			if contains(briefCodes, val["brief"].(string)) == true {
				e.db.deleteRecord2(TblName.Consent, "token", userTOKEN, "who", parsedData.phoneIdx)
			} else {
				briefCodes = append(briefCodes, val["brief"].(string))
			}
		}
	}
	event.Record = userTOKEN
	returnUUID(w, userTOKEN)
	notifyURL := e.conf.Notification.ProfileNotificationURL
	notifyProfileNew(notifyURL, parsedData.jsonData, "token", userTOKEN)
	return
}

func (e mainEnv) userGet(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	var err error
	var resultJSON []byte
	address := ps.ByName("address")
	mode := ps.ByName("mode")
	event := audit("get user record by "+mode, address, mode, address)
	defer func() { event.submit(e.db) }()
	if e.enforceAuth(w, r, event) == "" {
		return
	}
	if validateMode(mode) == false {
		returnError(w, r, "bad mode", 405, nil, event)
		return
	}
	userTOKEN := address
	if mode == "token" {
		if enforceUUID(w, address, event) == false {
			return
		}
		resultJSON, err = e.db.getUser(address)
	} else {
		resultJSON, userTOKEN, err = e.db.getUserIndex(address, mode, e.conf)
		event.Record = userTOKEN
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
	address := ps.ByName("address")
	mode := ps.ByName("mode")
	event := audit("change user record by "+mode, address, mode, address)
	defer func() { event.submit(e.db) }()

	authResult := e.enforceAuth(w, r, event)
	if authResult == "" {
		return
	}
	if validateMode(mode) == false {
		returnError(w, r, "bad index", 405, nil, event)
		return
	}
	if mode == "token" && enforceUUID(w, address, event) == false {
		return
	}
	parsedData, err := getJSONPost(r, e.conf.Sms.DefaultCountry)
	if err != nil {
		returnError(w, r, "failed to decode request body", 405, err, event)
		return
	}
	if len(parsedData.jsonData) == 0 {
		returnError(w, r, "empty body", 405, nil, event)
		return
	}
	userTOKEN := address
	if mode != "token" {
		userBson, err := e.db.lookupUserRecordByIndex(mode, address, e.conf)
		if err != nil {
			returnError(w, r, "internal error", 405, err, event)
			return
		}
		if userBson == nil {
			returnError(w, r, "internal error", 405, nil, event)
			return
		}
		userTOKEN = userBson["token"].(string)
		event.Record = userTOKEN
	}
	if authResult == "login" {
		event.Title = "User change-me request"
		if e.conf.SelfService.UserRecordChange == false {
			rtoken, err := e.db.saveUserRequest("change-me", userTOKEN, "", "", parsedData.jsonData)
			if err != nil {
				returnError(w, r, "internal error", 405, err, event)
				return
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(200)
			fmt.Fprintf(w, `{"status":"ok","result":"request-created","rtoken":"%s"}`, rtoken)
			return
		}
	}
	oldJSON, newJSON, err := e.db.updateUserRecord(parsedData.jsonData, userTOKEN, event, e.conf)
	if err != nil {
		returnError(w, r, "internal error", 405, err, event)
		return
	}
	returnUUID(w, userTOKEN)
	notifyURL := e.conf.Notification.ProfileNotificationURL
	notifyProfileChange(notifyURL, oldJSON, newJSON, "token", userTOKEN)
}

// user forgetme request comes here
func (e mainEnv) userDelete(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	address := ps.ByName("address")
	mode := ps.ByName("mode")
	event := audit("delete user record by "+mode, address, mode, address)
	defer func() { event.submit(e.db) }()

	if validateMode(mode) == false {
		returnError(w, r, "bad mode", 405, nil, event)
		return
	}
	var err error
	var resultJSON []byte
	userTOKEN := address
	if mode == "token" {
		if enforceUUID(w, address, event) == false {
			return
		}
		resultJSON, err = e.db.getUser(address)
	} else {
		resultJSON, userTOKEN, err = e.db.getUserIndex(address, mode, e.conf)
		event.Record = userTOKEN
	}
	if err != nil {
		returnError(w, r, "internal error", 405, nil, event)
		return
	}
	authResult := e.enforceAuth(w, r, event)
	if authResult == "" {
		return
	}
	if resultJSON == nil {
		returnError(w, r, "record not found", 405, nil, event)
		return
	}
	if authResult == "login" {
		event.Title = "User forget-me request"
		if e.conf.SelfService.ForgetMe == false {
			rtoken, err := e.db.saveUserRequest("forget-me", userTOKEN, "", "", nil)
			if err != nil {
				returnError(w, r, "internal error", 405, err, event)
				return
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(200)
			fmt.Fprintf(w, `{"status":"ok","result":"request-created","rtoken":"%s"}`, rtoken)
			return
		}
	}
	fmt.Printf("deleting user %s\n", userTOKEN)

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
	notifyURL := e.conf.Notification.ForgetmeNotificationURL
	notifyForgetMe(notifyURL, resultJSON, "token", userTOKEN)
}

func (e mainEnv) userLogin(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	address := ps.ByName("address")
	mode := ps.ByName("mode")
	event := audit("user login by "+mode, address, mode, address)
	defer func() { event.submit(e.db) }()

	if mode != "phone" && mode != "email" {
		returnError(w, r, "bad mode", 405, nil, event)
		return
	}
	userBson, err := e.db.lookupUserRecordByIndex(mode, address, e.conf)
	if err != nil {
		returnError(w, r, "internal error", 405, err, event)
		return
	}
	if userBson != nil {
		userTOKEN := userBson["token"].(string)
		event.Record = userTOKEN
		if address == "4444" || address == "test@paranoidguy.com" {
			// check if it is demo account.
			// the address is always 4444
			// no need to send any notifications
			e.db.generateDemoLoginCode(userTOKEN)
		} else {
			rnd := e.db.generateTempLoginCode(userTOKEN)
			if mode == "email" {
				go sendCodeByEmail(rnd, address, e.conf)
			} else if mode == "phone" {
				go sendCodeByPhone(rnd, address, e.conf)
			}
		}
	} else {
		fmt.Println("user record not found, still returning ok status")
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(200)
	fmt.Fprintf(w, `{"status":"ok","result":"done"}`)
}

func (e mainEnv) userLoginEnter(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	tmp := atoi(ps.ByName("tmp"))
	address := ps.ByName("address")
	mode := ps.ByName("mode")
	event := audit("user login2 by "+mode, address, mode, address)
	defer func() { event.submit(e.db) }()

	if mode != "phone" && mode != "email" {
		returnError(w, r, "bad mode", 405, nil, event)
		return
	}

	userBson, err := e.db.lookupUserRecordByIndex(mode, address, e.conf)
	if userBson == nil || err != nil {
		returnError(w, r, "internal error", 405, err, event)
		return
	}

	userTOKEN := userBson["token"].(string)
	event.Record = userTOKEN
	tmpCode := int32(0)
	if _, ok := userBson["tempcode"]; ok {
		tmpCode = userBson["tempcode"].(int32)
	}
	if tmp == tmpCode {
		// user ented correct key
		// generate temp user access code
		xtoken, err := e.db.generateUserLoginXtoken(userTOKEN)
		//fmt.Printf("generate user access token: %s\n", xtoken)
		if err != nil {
			returnError(w, r, "internal error", 405, err, event)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(200)
		fmt.Fprintf(w, `{"status":"ok","xtoken":"%s","token":"%s"}`, xtoken, userTOKEN)
		return
	}
	returnError(w, r, "internal error", 405, nil, event)
}
