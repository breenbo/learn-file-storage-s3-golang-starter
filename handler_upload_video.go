package main

import (
	"io"
	"mime"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	//
	// get the video UUID
	//
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	//
	// validate the user with the token in header
	//
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

	//
	// check if user owner of the DBvideo
	//
	DBvideo, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get video", err)
		return

	}
	if DBvideo.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "User not authorized to upload thumbnail for this video", err)
		return
	}

	//
	// parse the video file from the input form
	//
	const maxMemory = 10 << 30 // Max memory to 1 Gb
	err = r.ParseMultipartForm(maxMemory)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't parse multipart form for video", err)
		return
	}

	video, handler, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't get video file", err)
		return
	}

	//
	// prevent memory leak by closing when not usefull anymore
	//
	defer video.Close()

	//
	// validate the mime type
	//
	mimeType, _, err := mime.ParseMediaType(handler.Header.Get("Content-type"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't parse mime type", err)
		return
	}
	if mimeType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "invalid video format", err)
		return
	}

	//
	// save in temp file
	//
	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create temp file", err)
		return
	}
	defer os.Remove("tubely-upload.mp4")
	defer tempFile.Close()

	_, err = io.Copy(tempFile, video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create temp file", err)
		return
	}

	//
	// read the file from the begining (after copy, pointer to the end of the file)
	//
	_, err = tempFile.Seek(0, io.SeekStart)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error with temp file", err)
		return
	}

	//
	// Send the video to S3
	//
	key := getAssetPath(mimeType)
	putObjectInput := s3.PutObjectInput{
		Bucket:      aws.String(cfg.s3Bucket),
		Key:         aws.String(key),
		Body:        tempFile,
		ContentType: aws.String(mimeType),
	}
	_, err = cfg.s3Client.PutObject(r.Context(), &putObjectInput)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't upload video to S3", err)
		return
	}

	//
	// update the video url in the db
	//
	url := cfg.getObjectURL(key)
	DBvideo.VideoURL = &url
	err = cfg.db.UpdateVideo(DBvideo)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video URL", err)
		return
	}

	//
	// respond with the video
	//
	respondWithJSON(w, http.StatusOK, DBvideo)
}
