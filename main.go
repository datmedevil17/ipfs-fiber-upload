package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	

	"github.com/gofiber/fiber/v2"
	"github.com/joho/godotenv"
)

type PinataResponse struct {
	IpfsHash string `json:"IpfsHash"`
}

func loadEnv() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("❌ Error loading .env file")
	}
}

func uploadToIPFS(file multipart.File, fileHeader *multipart.FileHeader) (string, error) {
	pinataAPIKey := os.Getenv("PINATA_API_KEY")
	pinataSecret := os.Getenv("PINATA_SECRET_API_KEY")

	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)

	part, err := writer.CreateFormFile("file", fileHeader.Filename)
	if err != nil {
		return "", err
	}

	_, err = io.Copy(part, file)
	if err != nil {
		return "", err
	}
	writer.Close()

	req, err := http.NewRequest("POST", "https://api.pinata.cloud/pinning/pinFileToIPFS", &requestBody)
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("pinata_api_key", pinataAPIKey)
	req.Header.Set("pinata_secret_api_key", pinataSecret)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("pinata error: %s", string(body))
	}

	var pinataRes PinataResponse
	err = json.Unmarshal(body, &pinataRes)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("https://ipfs.io/ipfs/%s", pinataRes.IpfsHash), nil
}

func startFiberApp(wg *sync.WaitGroup) {
	defer wg.Done()
	app := fiber.New()

	app.Post("/upload", func(c *fiber.Ctx) error {
		fileHeader, err := c.FormFile("file")
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "File missing"})
		}

		file, err := fileHeader.Open()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "File open failed"})
		}
		defer file.Close()

		ipfsURL, err := uploadToIPFS(file, fileHeader)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(fiber.Map{
			"ipfs_url": ipfsURL,
		})
	})

	fmt.Println("🚀 Server started at http://localhost:3000")
	log.Fatal(app.Listen(":3000"))
}

func cliUpload() {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("Enter the path of the image file (or 'exit' to quit): ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input == "exit" {
			fmt.Println("Exiting CLI uploader.")
			break
		}

		file, err := os.Open(input)
		if err != nil {
			fmt.Println("Error opening file:", err)
			continue
		}

		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)

		part, err := writer.CreateFormFile("file", filepath.Base(input))
		if err != nil {
			fmt.Println("Error creating form file:", err)
			file.Close()
			continue
		}

		_, err = io.Copy(part, file)
		if err != nil {
			fmt.Println("Error copying file:", err)
			file.Close()
			continue
		}
		file.Close()
		writer.Close()

		req, err := http.NewRequest("POST", "http://localhost:3000/upload", body)
		if err != nil {
			fmt.Println("Error creating request:", err)
			continue
		}
		req.Header.Set("Content-Type", writer.FormDataContentType())

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			fmt.Println("Upload failed:", err)
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			fmt.Println("Error reading response:", err)
			continue
		}

		fmt.Println("Response from server:", string(respBody))
	}
}

func main() {
	loadEnv()

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "server":
			// Run only the Fiber web server
			var wg sync.WaitGroup
			wg.Add(1)
			go startFiberApp(&wg)
			wg.Wait() // will block forever
		case "cli":
			// Run only CLI uploader, assumes server is running on localhost:3000
			cliUpload()
		default:
			fmt.Println("Unknown argument. Use 'server' or 'cli'")
		}
		return
	}

	// Default: run both server and CLI uploader in one process
	var wg sync.WaitGroup
	wg.Add(1)
	go startFiberApp(&wg)

	// Wait for server to start
	time.Sleep(1 * time.Second)

	cliUpload()

	wg.Wait()
}
