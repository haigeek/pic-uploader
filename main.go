package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config 结构体
type Config struct {
	APIUrl   string `yaml:"api_url"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// ApiResponse API响应结构体
type ApiResponse struct {
	Status int    `json:"status"`
	Code   int    `json:"code"`
	Msg    string `json:"msg"`
	Data   string `json:"data"`
}

// 上传结果结构体
type UploadResult struct {
	FilePath string // 原始文件路径
	ImageURL string // 上传后的URL
	Error    error  // 错误信息
}

func main() {
	// 解析命令行参数
	var configFile string
	flag.StringVar(&configFile, "config", "typora-upload-config.yaml", "Path to config file")
	flag.Parse()

	// 获取图片路径参数
	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: typora-upload [--config=<path>] <image-path1> <image-path2> ...")
		fmt.Fprintln(os.Stderr, "Default config file: typora-upload-config.yaml")
		os.Exit(1)
	}

	// 加载配置
	config, err := loadConfig(configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// 处理所有图片上传
	results := uploadImages(config, args)

	// 输出结果
	hasError := false
	for _, result := range results {
		if result.Error != nil {
			fmt.Fprintf(os.Stderr, "Upload failed for %s: %v\n", result.FilePath, result.Error)
			hasError = true
		} else {
			// fmt.Printf("![](%s)\n", result.ImageURL)
			fmt.Println(result.ImageURL)
		}
	}

	if hasError {
		os.Exit(1)
	}
}

// loadConfig 从YAML文件加载配置
func loadConfig(configFile string) (Config, error) {
	var config Config

	// 读取配置文件
	data, err := os.ReadFile(configFile)
	if err != nil {
		return config, fmt.Errorf("failed to read config file: %v", err)
	}

	// 解析YAML
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return config, fmt.Errorf("failed to parse config file: %v", err)
	}

	// 验证必要配置
	if config.APIUrl == "" {
		return config, fmt.Errorf("api_url is required in config")
	}
	if config.Username == "" || config.Password == "" {
		return config, fmt.Errorf("username and password are required in config")
	}

	return config, nil
}

// uploadImages 上传多个图片到服务器
func uploadImages(config Config, imagePaths []string) []UploadResult {
	results := make([]UploadResult, len(imagePaths))
	ch := make(chan UploadResult, len(imagePaths))

	// 并发上传所有图片
	for _, path := range imagePaths {
		go func(p string) {
			url, err := uploadImage(config, p)
			ch <- UploadResult{
				FilePath: p,
				ImageURL: url,
				Error:    err,
			}
		}(path)
	}

	// 收集结果
	for i := 0; i < len(imagePaths); i++ {
		results[i] = <-ch
	}

	return results
}

// uploadImage 上传单个图片到服务器
func uploadImage(config Config, imagePath string) (string, error) {
	// 打开图片文件
	file, err := os.Open(imagePath)
	if err != nil {
		return "", fmt.Errorf("failed to open image: %v", err)
	}
	defer file.Close()

	// 创建multipart表单
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// 获取文件名
	filename := filepath.Base(imagePath)

	// 获取文件扩展名并设置Content-Type
	contentType := getContentType(imagePath)

	// 创建表单文件部分
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", fmt.Errorf("failed to create form file: %v", err)
	}

	// 复制文件内容
	_, err = io.Copy(part, file)
	if err != nil {
		return "", fmt.Errorf("failed to copy file content: %v", err)
	}

	// 添加headers
	headers := fmt.Sprintf("Content-Type: %s", contentType)
	writer.WriteField("headers", headers)
	writer.Close()

	// 创建HTTP请求
	req, err := http.NewRequest("POST", config.APIUrl, body)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}

	// 设置Basic Auth和Content-Type
	req.SetBasicAuth(config.Username, config.Password)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// 解析响应
	return parseResponse(resp)
}

// getContentType 根据文件扩展名获取Content-Type
func getContentType(imagePath string) string {
	ext := strings.ToLower(filepath.Ext(imagePath))
	if ext != "" && ext[0] == '.' {
		ext = ext[1:]
	}

	switch ext {
	case "jpg", "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "gif":
		return "image/gif"
	case "svg":
		return "image/svg+xml"
	case "webp":
		return "image/webp"
	default:
		return "image/" + ext
	}
}

// parseResponse 解析API响应
func parseResponse(resp *http.Response) (string, error) {
	// 读取响应
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %v", err)
	}

	// 解析JSON响应
	var apiResp ApiResponse
	err = json.Unmarshal(respBody, &apiResp)
	if err != nil {
		return "", fmt.Errorf("failed to parse response: %v", err)
	}

	// 检查状态码
	if apiResp.Status != 200 || apiResp.Code != 1 {
		return "", fmt.Errorf("upload failed: %s", apiResp.Msg)
	}

	return apiResp.Data, nil
}
