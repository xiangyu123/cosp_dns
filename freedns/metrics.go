package freedns

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/sirupsen/logrus"
)

const (
	TOKEN           = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ2ZXJzaW9uIjoxLCJhcHBjb2RlIjoib3BzX3N5c19vZmZpY2VkbnMiLCJ0b191c2VyIjoic29uZ2t1YW4uemhlbmciLCJpYXQiOjE2MzUzMTczMjR9.p31Yx0aDsTGGdfy6boP-xWme7MJmP82U4C2Ull1CBNw"
	AppCode         = "ops_sys_officedns"
	applicationJSON = "application/json;charset=utf-8"
	// ProdURL         = "http://metric-gateway.opsbeta.wormpex.com/v1/metric/send" // beta, 暂时用测试环境的
	ProdURL = "http://metric-gateway.vip.blibee.com/v1/metric/send"
)

type JsonData struct {
	Metrics []Metrics `json:"metrics"`
}

type Metrics struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value int    `json:"value"`
}

func countGraph() (s, f int) {
	for i := 0; i < len(ss); i++ {
		v := <-ss
		s += int(v)
	}

	for j := 0; j < len(ff); j++ {
		v := <-ff
		f += int(v)
	}

	return
}

func sendMetricsToServer(j JsonData) {
	requestBody, _ := json.Marshal(&j)
	log.Info("send..", string(requestBody))
	req, _ := http.NewRequest("POST", ProdURL, bytes.NewBuffer(requestBody))
	req.Header.Set("Content-Type", applicationJSON)
	req.Header.Add("X-App-Token", TOKEN)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.WithFields(logrus.Fields{
			"task":   "sendMetricsToServer",
			"status": err.Error(),
		}).Error()
	} else {
		log.WithFields(logrus.Fields{
			"task":   "sendMetricsToServer",
			"status": resp.Status,
		}).Error()
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Error("调用接口异常%v", err.Error())
	} else {
		log.Info(string(body))
	}
}

func buildMetrics(s, f int) (j JsonData) {
	metricsData := make([]Metrics, 0)
	succMetric := Metrics{
		Type:  "counter",
		Name:  fmt.Sprintf("requests.%s.success.count", hostName),
		Value: s,
	}

	failMetric := Metrics{
		Type:  "counter",
		Name:  fmt.Sprintf("requests.%s.faild.count", hostName),
		Value: f,
	}

	metricsData = append(metricsData, succMetric, failMetric)

	j = JsonData{metricsData}
	// return json data
	return
}

// 一分钟唤醒一次这个作业
func collectMetrics() {
	sCount, fCount := countGraph()
	metricsJson := buildMetrics(sCount, fCount)

	// send
	sendMetricsToServer(metricsJson)
}
