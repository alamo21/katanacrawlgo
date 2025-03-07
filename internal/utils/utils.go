package utils

import (
	"bufio"
	"encoding/json"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/ttacon/chalk"
)

type Record struct {
	Originalurl string `json:"Originalurl"`
	Response    string `json:"Response"`
}

// 处理URL端口
func processURLPort(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}

	// 处理端口逻辑
	switch {
	case u.Scheme == "http" && u.Port() == "80":
		u.Host = u.Hostname()
	case u.Scheme == "https" && u.Port() == "443":
		u.Host = u.Hostname()
	}

	return u.String(), nil
}

func UniqueUrls(strSlice []string) []string {
	keys := make(map[string]bool)
	list := []string{}
	for _, entry := range strSlice {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list

}

func AppendToFile(filename string, text string) {
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Println(chalk.Red.Color("error: " + filename + "文件创建/打开失败, " + err.Error()))
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	_, err = w.WriteString(text + "\n")
	if err != nil {
		log.Println(chalk.Red.Color("error: " + text + "写入" + filename + "文件失败, " + err.Error()))
	}
	w.Flush()
}
func GetUrlListFromTxt(txtPath string) []string {

	var txtlines []string
	if txtPath != "" {
		file, err := os.Open(txtPath)
		if err != nil {
			log.Println(chalk.Red.Color("error: failed to open file: " + err.Error()))
		}
		scanner := bufio.NewScanner(file)
		scanner.Split(bufio.ScanLines)
		for scanner.Scan() {
			txtlines = append(txtlines, scanner.Text())
		}
		file.Close()
	}
	return txtlines
}

func GetUrlListFromPortTxt(txtPath string) []string {

	var txtlines []string
	if txtPath != "" {
		file, err := os.Open(txtPath)
		if err != nil {
			log.Println(chalk.Red.Color("error: failed to open file: " + err.Error()))
		}
		// 读取文件内容
		content, err2 := os.ReadFile(txtPath)
		if err2 != nil {
			log.Println(chalk.Red.Color("error: failed to read file content: " + err2.Error()))
			return txtlines
		}
		var records []Record
		err = json.Unmarshal(content, &records)
		if err != nil {
			log.Println(chalk.Red.Color("error: failed to parse file content to json: " + err.Error()))
			return txtlines
		}
		for _, record := range records {
			if record.Originalurl == "" {
				continue
			}
			// 检查是否包含:和http
			if !strings.Contains(record.Originalurl, ":") || !(strings.Contains(record.Originalurl, "http://") || strings.Contains(record.Originalurl, "https://")) {
				continue
			}
			if record.Response == "" || len(record.Response) <= 10 || strings.Contains(record.Response, "400 Bad Request\r\nDate") {
				continue
			}
			// 处理URL端口
			processedURL, err3 := processURLPort(record.Originalurl)
			if err3 != nil {
				continue // 跳过无效URL
			}
			txtlines = append(txtlines, processedURL)
		}
		file.Close()
	}
	return txtlines
}
