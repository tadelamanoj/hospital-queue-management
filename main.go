package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-xorm/xorm"
	_ "github.com/mattn/go-sqlite3"
)

//TODO: 只允许一个客户端进行连接
func main() {
	os.MkdirAll("./database", os.ModeDir|os.ModePerm)
	r := gin.Default()
	backend := NewMaster()

	r.StaticFile("/", "./web/panel/index.html")
	r.StaticFile("/controller", "./web/controller/index.html")
	r.Static("/js", "./web/js")
	r.Static("/css", "./web/css")
	r.Static("/ads/img", "./web/ads/img")

	r.GET("/patient_list", backend.GetPatientList)
	r.POST("/patient_list", backend.PostPatientList)
	r.DELETE("/patient_list", backend.DeletePatientList)

	r.DELETE("/patient_list/:id", backend.DeletePatient)
	r.PUT("/patient_list/:id/actions/call", backend.CallPatient)
	r.PUT("/patient_list/:id/actions/move_up", backend.MoveUpPatient)
	r.PUT("/patient_list/:id/actions/move_down", backend.MoveDownPatient)

	r.GET("/call_patient", backend.GetCallPatient)

	r.GET("/ads_img", backend.GetAdvertisementsImages)
	r.GET("/ads_img/interval", backend.GetPicInterval)
	r.PUT("/ads_img/interval", backend.SetPicInterval)

	r.GET("/notification", backend.GetNotification)
	r.PUT("/notification", backend.SetNotification)
	r.Run()
}

/*
func onlyOneClient(c *gin.Context) {
	clientAddr := c.Request.RemoteAddr
}
*/

type Backend interface {
	GetPatientList(c *gin.Context)
	PostPatientList(c *gin.Context)
	DeletePatientList(c *gin.Context)
	DeletePatient(c *gin.Context)
	MoveUpPatient(c *gin.Context)
	MoveDownPatient(c *gin.Context)
	CallPatient(c *gin.Context)
	GetCallPatient(c *gin.Context)

	GetAdvertisementsImages(c *gin.Context)

	SetPicInterval(c *gin.Context)
	GetPicInterval(c *gin.Context)

	GetNotification(c *gin.Context)
	SetNotification(c *gin.Context)
}

type Master struct {
	mutex        sync.Mutex
	db           *xorm.Engine
	callPatient  *WaitingPatient
	picInterval  int
	notification string
}

func NewMaster() Backend {
	m := &Master{}
	db, err := xorm.NewEngine("sqlite3", "./database/db.sqlite")
	if err != nil {
		fmt.Println("errrorororo: ", err)
		os.Exit(1)
	}
	exist, err := db.IsTableExist(WaitingPatient{})
	if err != nil {
		fmt.Println("Errrrrr check tabel: ", err)
	}
	if !exist {
		fmt.Println("table not exist")
		err := db.CreateTables(WaitingPatient{})
		if err != nil {
			fmt.Println("Errrrrr create tabel: ", err)
		}
	}
	m.db = db
	m.picInterval = 10
	m.notification = "祝您身体健康!"
	return m
}

func (m *Master) GetPatientList(c *gin.Context) {
	patients := []WaitingPatient{}
	err := m.db.Asc("id").Find(&patients)
	if err != nil {
		fmt.Println("find patiends error: ", err)
		c.JSON(400, "")
		return
	}
	c.JSON(200, patients)
}

func (m *Master) PostPatientList(c *gin.Context) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	newPatients := []WaitingPatient{}
	err := c.ShouldBind(&newPatients)
	if err != nil {
		fmt.Println("errr binding: ", err)
		c.JSON(400, "")
		return
	}
	if len(newPatients) == 0 {
		fmt.Println("no one to submit")
		c.JSON(200, "")
		return
	}
	session := m.db.NewSession()
	defer session.Close()
	err = session.Begin()
	if err != nil {
		fmt.Println("add patients fail")
		c.JSON(400, "")
		return
	}
	var firstAddPatient WaitingPatient
	for i, newPatient := range newPatients {
		_, err = session.Insert(&newPatient)
		if err != nil {
			session.Rollback()
			fmt.Println("insert err: ", err)
			c.JSON(400, "")
			return
		}
		if i == 0 {
			firstAddPatient = newPatient
		}
	}
	err = session.Commit()
	if err != nil {
		fmt.Println("add patients fail")
		c.JSON(400, "")
		return
	}
	if m.IsFirstPatient(firstAddPatient.Id) {
		m.callPatient = &firstAddPatient
	}
	c.JSON(200, "")
}

func (m *Master) DeletePatientList(c *gin.Context) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	_, err := m.db.Exec("delete from waiting_patient")
	if err != nil {
		fmt.Println("delete err: ", err)
	}
	c.JSON(200, "")
}

func (m *Master) UpdatePatient(c *gin.Context) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, "")
		return
	}
	newPatient := WaitingPatient{}
	err = c.ShouldBind(&newPatient)
	if err != nil {
		fmt.Println("errr binding: ", err)
		c.JSON(400, "")
		return
	}
	fmt.Println("get new patient:", newPatient)
	_, err = m.db.ID(id).Update(&newPatient)
	if err != nil {
		fmt.Println("delete err: ", err)
		c.JSON(400, "")
		return
	}
	if m.IsFirstPatient(id) {
		m.callPatient = &newPatient
	}
	c.JSON(200, "")
}

func (m *Master) CallPatient(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, "")
		return
	}
	patient := WaitingPatient{}
	has, err := m.db.ID(id).Get(&patient)
	if err != nil {
		fmt.Println("no this patient: ", err)
		c.JSON(200, "")
		return
	}
	if !has {
		fmt.Println("no this patient")
		c.JSON(200, "")
		return
	}
	m.callPatient = &patient
	c.JSON(200, "")
}

func (m *Master) DeletePatient(c *gin.Context) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, "")
		return
	}
	isFirstPatient := m.IsFirstPatient(id)
	n, err := m.db.ID(id).Delete(&WaitingPatient{})
	if err != nil {
		fmt.Println("delete err: ", err)
	}
	if isFirstPatient {
		newFirst := m.GetFirstPatient()
		if newFirst != nil {
			m.callPatient = newFirst
		}
	}
	c.JSON(200, n)
}

func (m *Master) MoveUpPatient(c *gin.Context) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, "")
		return
	}
	patient := WaitingPatient{}
	has, err := m.db.ID(id).Get(&patient)
	if err != nil {
		fmt.Println("no this patient: ", err)
		c.JSON(400, "")
		return
	}
	if !has {
		fmt.Println("no this patient")
		c.JSON(400, "")
		return
	}
	prePatient := WaitingPatient{}
	has, err = m.db.Where("id < ?", id).Desc("id").Limit(1, 0).Get(&prePatient)
	if err != nil {
		fmt.Println("get pre patient err: ", err)
		c.JSON(400, "")
		return
	}
	if !has {
		fmt.Println("no pre patient")
		c.JSON(200, "")
		return
	}
	session := m.db.NewSession()
	defer session.Close()
	err = session.Begin()
	if err != nil {
		fmt.Println("move up failed")
		c.JSON(400, "")
		return
	}
	_, err = session.ID(prePatient.Id).Update(&patient)
	if err != nil {
		session.Rollback()
		fmt.Println("move up failed")
		c.JSON(400, "")
		return
	}
	_, err = session.ID(patient.Id).Update(&prePatient)
	if err != nil {
		session.Rollback()
		fmt.Println("move up failed")
		c.JSON(400, "")
		return
	}
	err = session.Commit()
	if err != nil {
		fmt.Println("move down failed")
		c.JSON(400, "")
		return
	}
	if m.IsFirstPatient(prePatient.Id) {
		// NOTE: id is incorrect, if need to use id, re-struct it
		m.callPatient = &patient
	}
	c.JSON(200, "")
}

func (m *Master) MoveDownPatient(c *gin.Context) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, "")
		return
	}
	patient := WaitingPatient{}
	has, err := m.db.ID(id).Get(&patient)
	if err != nil {
		fmt.Println("no this patient: ", err)
		c.JSON(400, "")
		return
	}
	if !has {
		fmt.Println("no this patient")
		c.JSON(400, "")
		return
	}
	nextPatient := WaitingPatient{}
	has, err = m.db.Where("id > ?", id).Asc("id").Limit(1, 0).Get(&nextPatient)
	if err != nil {
		fmt.Println("get next patient err: ", err)
		c.JSON(400, "")
		return
	}
	if !has {
		fmt.Println("no next patient")
		c.JSON(400, "")
		return
	}
	fmt.Println("next patient", nextPatient.Name)
	session := m.db.NewSession()
	defer session.Close()
	err = session.Begin()
	if err != nil {
		fmt.Println("move down failed")
		c.JSON(400, "")
		return
	}
	n, err := session.ID(nextPatient.Id).Update(&patient)
	if err != nil {
		session.Rollback()
		fmt.Println("move down failed")
		c.JSON(400, "")
		return
	}
	fmt.Println("update ", n)

	_, err = session.ID(patient.Id).Update(&nextPatient)
	if err != nil {
		session.Rollback()
		fmt.Println("move down failed")
		c.JSON(400, "")
		return
	}
	err = session.Commit()
	if err != nil {
		fmt.Println("move down failed")
		c.JSON(400, "")
		return
	}
	if m.IsFirstPatient(patient.Id) {
		// NOTE: id is incorrect, if need to use id, re-struct it
		m.callPatient = &nextPatient
	}
	c.JSON(200, "")
}

func (m *Master) GetCallPatient(c *gin.Context) {
	callPatient := m.callPatient
	m.callPatient = nil
	c.JSON(200, callPatient)
}

func (m *Master) GetAdvertisementsImages(c *gin.Context) {
	files, err := filepath.Glob("web/ads/img/*")
	if err != nil {
		c.JSON(200, "")
		return
	}
	res := []string{}
	for _, file := range files {
		fileName := strings.TrimLeft(file, "web/")
		res = append(res, fileName)
	}
	c.JSON(200, res)
}

func (m *Master) GetPicInterval(c *gin.Context) {
	c.JSON(200, m.picInterval)
}

type PicInterval struct {
	Interval int `json:"interval"`
}

func (m *Master) SetPicInterval(c *gin.Context) {
	picInterval := PicInterval{}
	if c.ShouldBindJSON(&picInterval) != nil {
		c.JSON(400, "")
		return
	}
	fmt.Println("set pic interval to: ", picInterval.Interval)
	m.picInterval = picInterval.Interval
	c.JSON(200, picInterval.Interval)
}

type Notification struct {
	Content string `json:"content"`
}

func (m *Master) SetNotification(c *gin.Context) {
	noti := Notification{}
	if c.ShouldBindJSON(&noti) != nil {
		c.JSON(400, "")
		return
	}
	fmt.Println("set notificaiton to : ", noti.Content)
	m.notification = noti.Content
	c.JSON(200, noti.Content)
}

func (m *Master) GetNotification(c *gin.Context) {
	c.JSON(200, m.notification)
}

func (m *Master) IsFirstPatient(id int64) bool {
	prePatient := WaitingPatient{}
	has, err := m.db.Where("id < ?", id).Desc("id").Limit(1, 0).Get(&prePatient)
	if err != nil {
		fmt.Println("get pre patient err: ", err)
		return false
	}
	if has {
		return false
	}
	return true
}

func (m *Master) GetFirstPatient() *WaitingPatient {
	firstPatient := WaitingPatient{}
	has, err := m.db.Asc("id").Limit(1, 0).Get(&firstPatient)
	if err != nil {
		fmt.Println("get first patient err: ", err)
		return nil
	}
	if !has {
		fmt.Println("no patient")
		return nil
	}
	return &firstPatient
}

type WaitingPatient struct {
	Id         int64     `json:"id"`
	Name       string    `json:"name"`
	Uid        string    `json:"uid"`
	ClinicNum  string    `json:"clinic_num"`
	CreateTime time.Time `xorm:"created" json:"create_time"`
	UpdateTime time.Time `xorm:"updated" json:"update_time"`
}
