package main

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"

	uuid "github.com/hashicorp/go-uuid"
	"go.mongodb.org/mongo-driver/bson"
)

type sessionEvent struct {
	When int32
	Meta []byte
}

func (dbobj dbcon) generateUserSession(userTOKEN string, clientip string, expiration string, meta []byte) (string, error) {
	if len(expiration) == 0 {
		return "", errors.New("failed to parse expiration")
	}
	endtime, err := parseExpiration(expiration)
	if err != nil {
		return "", err
	}
	encodedStr, err := dbobj.userEncrypt(userTOKEN, meta)
	if err != nil {
		return "", err
	}
	tokenUUID, err := uuid.GenerateUUID()
	if err != nil {
		return "", err
	}
	bdoc := bson.M{}
	bdoc["token"] = userTOKEN
	bdoc["session"] = tokenUUID
	bdoc["endtime"] = endtime
	bdoc["meta"] = encodedStr
	if len(clientip) > 0 {
		idxString := append(dbobj.hash, []byte(clientip)...)
		idxStringHash := sha256.Sum256(idxString)
		bdoc["clientipidx"] = base64.StdEncoding.EncodeToString(idxStringHash[:])
	}
	_, err = dbobj.createRecord(TblName.Sessions, bdoc)
	if err != nil {
		return "", err
	}
	return tokenUUID, nil
}

func (dbobj dbcon) getUserSession(sessionUUID string) ([]byte, error) {
	record, err := dbobj.getRecord(TblName.Sessions, "session", sessionUUID)
	if record == nil || err != nil {
		return nil, errors.New("failed to authenticate")
	}
	// check expiration
	now := int32(time.Now().Unix())
	if now > record["endtime"].(int32) {
		return nil, errors.New("session expired")
	}
	userTOKEN := record["token"].(string)
	encData0 := record["meta"].(string)
	decrypted, err := dbobj.userDecrypt(userTOKEN, encData0)
	if err != nil {
		return nil, err
	}
	return decrypted, err
}

func (dbobj dbcon) getUserSessionByToken(userTOKEN string) ([]*sessionEvent, int64, error) {

	userBson, err := dbobj.lookupUserRecord(userTOKEN)
	if userBson == nil || err != nil {
		// not found
		return nil, 0, err
	}
	userKey := userBson["key"].(string)
	recordKey, err := base64.StdEncoding.DecodeString(userKey)
	if err != nil {
		return nil, 0, err
	}

	count, err := dbobj.countRecords(TblName.Sessions, "token", userTOKEN)
	if err != nil {
		return nil, 0, err
	}

	records, err := dbobj.getList(TblName.Sessions, "token", userTOKEN, 0, 0)
	if err != nil {
		return nil, 0, err
	}

	var results []*sessionEvent
	for _, element := range records {
		encData0 := element["meta"].(string)
		encData, _ := base64.StdEncoding.DecodeString(encData0)
		decrypted, _ := decrypt(dbobj.masterKey, recordKey, encData)
		sEvent := sessionEvent{0, decrypted}
		results = append(results, &sEvent)
	}

	return results, count, err
}