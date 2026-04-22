package main

import (
	"os"
	"io"
	"net/http"
	"mime"
	"bytes"
	"math"
	"fmt"
	"errors"
	"os/exec"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	// Set Upload Limit
	const uploadLimit = 1 << 30
	r.Body = http.MaxBytesReader(w, r.Body, uploadLimit)

	// Extract videoID
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	// Authenticate User
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}
	
	// Get video metadata
	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't find video", err)
		return
	}

	// Check if User is video owner
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Not authorized to upload this video", err)
		return
	}

	// Get video data from the form
	file, fileHeader, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	// Get Media Type
	mediaType, _, err := mime.ParseMediaType(fileHeader.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Content-Type", err)
		return
	}

	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Invalid file type", err)
		return
	}

	// Save uploaded file to disk (Temporary)
	dst, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create temp file", err)
		return
	}
	defer os.Remove(dst.Name())
	defer dst.Close()

	if _, err = io.Copy(dst, file); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error saving file", err)
		return
	}
	if _, err = dst.Seek(0, io.SeekStart); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error resetting pointer to start of file", err)
		return
	}

	// Get Aspect Ratio
	ratio, err := getVideoAspectRatio(dst.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error getting video aspect ratio", err)
		return
	}

	// Upload Video to S3
	key := getAssetPath(mediaType)

	if ratio == "16:9" {
		key = "landscape/" + key
	} else if ratio == "9:16" {
		key = "portrait/" + key
	} else {
		key = "other/" + key
	}

	s3PutObjectInput := s3.PutObjectInput{
		Bucket:	aws.String(cfg.s3Bucket),
		Key: aws.String(key),
		Body: dst,
		ContentType: aws.String(mediaType),
	}
	_, err = cfg.s3Client.PutObject(r.Context(), &s3PutObjectInput)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to upload to S3 server", err)
		return
	}

	// Update VideoURL
	url := cfg.getObjectURL(key)
	video.VideoURL = &url

	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}

func getVideoAspectRatio(filePath string) (string, error) {
	type FFProbe struct {
		Streams []struct {
			Width              int    `json:"width,omitempty"`
			Height             int    `json:"height,omitempty"`
		} `json:"streams"`
	}

	args := []string{
		"-v",
		"error",
		"-print_format",
		"json",
		"-show_streams",
		filePath,
	}
	cmd := exec.Command("ffprobe", args...)

	var buff bytes.Buffer
	cmd.Stdout = &buff

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ffprobe error: %v", err)
	}

	var ffprobe FFProbe
	if err := json.Unmarshal(buff.Bytes(), &ffprobe); err != nil {
		return "", fmt.Errorf("Couldn't parse ffprobe output: %v", err)
	}

	if len(ffprobe.Streams) == 0 {
		return "", errors.New("No video streams found")
	}

	width := ffprobe.Streams[0].Width
	height := ffprobe.Streams[0].Height
	ratio := float64(width) / float64(height)
	landscape := 16.0 / 9.0
	portrait := 9.0 / 16.0
	if math.Abs(ratio - landscape) < 0.01 {
		return "16:9", nil
	}
	if math.Abs(ratio - portrait) < 0.01 {
		return "9:16", nil
	}
	
	return "other", nil
}
