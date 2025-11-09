package main

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

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

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	// max memory to 10 Mo
	const maxMemory = 10 << 20
	err = r.ParseMultipartForm(maxMemory)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't parse multipart form for thumbnail", err)
		return
	}

	image, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't get thumbnail file", err)
		return
	}

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get video", err)
		return
	}
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "User not authorized to upload thumbnail for this video", err)
		return
	}

	// store the image on the filesysytem
	mimeType, _, err := mime.ParseMediaType(header.Header.Get("Content-type"))
	if mimeType != "image/jpeg" && mimeType != "image/png" {
		respondWithError(w, http.StatusBadRequest, "invalid image", err)
		return
	}

	fileExtension := strings.Split(header.Header.Get("Content-type"), "/")[1]
	thumbName := fmt.Sprintf("%s.%s", video.ID, fileExtension)
	thumbPath := filepath.Join(cfg.assetsRoot, thumbName)

	emptyFile, err := os.Create(thumbPath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to create the file", err)
		return
	}
	_, err = io.Copy(emptyFile, image)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to copy the file", err)
		return
	}

	// update the video metadata in the DB
	dataURL := fmt.Sprintf("http://localhost:%s/assets/%s", cfg.port, thumbName)
	video.ThumbnailURL = &dataURL

	err = cfg.db.UpdateVideo(video)
	if err != nil {
		// remove the thumb from the map
		respondWithError(w, http.StatusInternalServerError, "Unable to update the video", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}
