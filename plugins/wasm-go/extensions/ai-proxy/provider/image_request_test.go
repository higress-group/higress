package provider

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestImageRequestsCollectAllURLs(t *testing.T) {
	images := []imageInputURL{
		{URL: "https://example.com/first.png"},
		{ImageURL: &chatMessageContentImageUrl{Url: "https://example.com/second.png"}},
	}
	image := &imageInputURL{URL: "https://example.com/image.png"}
	imageURL := &imageInputURL{URL: "https://example.com/image-url.png"}
	expected := []string{
		"https://example.com/first.png",
		"https://example.com/second.png",
		"https://example.com/image.png",
		"https://example.com/image-url.png",
	}

	editRequest := &imageEditRequest{
		Images:   images,
		Image:    image,
		ImageURL: imageURL,
	}
	require.Equal(t, expected, editRequest.GetImageURLs())

	variationRequest := &imageVariationRequest{
		Images:   images,
		Image:    image,
		ImageURL: imageURL,
	}
	require.Equal(t, expected, variationRequest.GetImageURLs())
}

func TestBuildVertexImageRequestKeepsImageAndPromptParts(t *testing.T) {
	request, err := (&vertexProvider{}).buildVertexImageRequest(
		"draw a gateway",
		"1024x1024",
		"png",
		[]string{"https://example.com/input.jpg"},
	)
	require.NoError(t, err)
	require.Len(t, request.Contents, 1)
	require.Len(t, request.Contents[0].Parts, 2)
	require.Equal(t, "https://example.com/input.jpg", request.Contents[0].Parts[0].FileData.FileUri)
	require.Equal(t, "draw a gateway", request.Contents[0].Parts[1].Text)
}
