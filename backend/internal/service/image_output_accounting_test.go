package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAIImageOutputAccountingIgnoresTextOnlyResponsesJSONData(t *testing.T) {
	body := []byte(`{
		"id": "resp_1",
		"object": "response",
		"data": [
			{"type": "message", "content": [{"type": "output_text", "text": "hello"}], "size": "4096x4096"},
			{"type": "usage", "input_tokens": 3, "output_tokens": 5}
		],
		"output": [
			{"id": "msg_1", "type": "message", "content": [{"type": "output_text", "text": "done"}]}
		]
	}`)

	require.Zero(t, countOpenAIResponseImageOutputsFromJSONBytes(body))
	require.Nil(t, collectOpenAIResponseImageOutputSizesFromJSONBytes(body))
}

func TestOpenAIImageOutputAccountingIgnoresTextOnlyResponsesSSEData(t *testing.T) {
	body := "data: {\"type\":\"response.output_text.delta\",\"data\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"hello\"}],\"size\":\"4096x4096\"}]}\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"id\":\"msg_1\",\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"done\"}]}]}}\n\n" +
		"data: [DONE]\n\n"

	require.Zero(t, countOpenAIImageOutputsFromSSEBody(body))
	require.Nil(t, collectOpenAIImageOutputSizesFromSSEBody(body))
}

func TestOpenAIImageOutputAccountingCountsImagesAPIDataAndSizes(t *testing.T) {
	body := []byte(`{
		"created": 1710000000,
		"data": [
			{"url": "https://example.test/image-a.png", "size": "1024x1024"},
			{"b64_json": "final-b", "size": "2048x1152"},
			{"revised_prompt": "not an image output", "size": "4096x4096"}
		]
	}`)

	require.Equal(t, 2, countOpenAIResponseImageOutputsFromJSONBytes(body))
	require.Equal(t, []string{"1024x1024", "2048x1152"}, collectOpenAIResponseImageOutputSizesFromJSONBytes(body))
}

func TestOpenAIImageOutputAccountingCountsRealImageGenerationCompleted(t *testing.T) {
	body := "data: {\"type\":\"image_generation.completed\",\"id\":\"ig_1\",\"b64_json\":\"final-a\",\"size\":\"1024x1024\"}\n\n" +
		"data: [DONE]\n\n"

	require.Equal(t, 1, countOpenAIImageOutputsFromSSEBody(body))
	require.Equal(t, []string{"1024x1024"}, collectOpenAIImageOutputSizesFromSSEBody(body))
}

func TestOpenAIImageOutputAccountingIgnoresEmptyImageGenerationCompleted(t *testing.T) {
	body := "data: {\"type\":\"image_generation.completed\",\"id\":\"ig_1\"}\n\n" +
		"data: [DONE]\n\n"

	require.Zero(t, countOpenAIImageOutputsFromSSEBody(body))
	require.Nil(t, collectOpenAIImageOutputSizesFromSSEBody(body))
}
