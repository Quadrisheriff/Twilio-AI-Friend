package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/sashabaranov/go-openai"
	"github.com/twilio/twilio-go/twiml"
	"io"
	"os"

	"log"
	"net/http"
)

type Transcripts struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Request struct {
	ResponseID      int           `json:"response_id"`
	Transcript      []Transcripts `json:"transcript"`
	InteractionType string        `json:"interaction_type"`
}

type Response struct {
	ResponseID      int    `json:"response_id"`
	Content         string `json:"content"`
	ContentComplete bool   `json:"content_complete"`
	EndCall         bool   `json:"end_call"`
}

type RegisterCallRequest struct {
	AgentID                string `json:"agent_id"`
	AudioEncoding          string `json:"audio_encoding"`
	AudioWebsocketProtocol string `json:"audio_websocket_protocol"`
	SampleRate             int    `json:"sample_rate"`
}

type RegisterCallResponse struct {
	AgentID                string `json:"agent_id"`
	AudioEncoding          string `json:"audio_encoding"`
	AudioWebsocketProtocol string `json:"audio_websocket_protocol"`
	CallID                 string `json:"call_id"`
	CallStatus             string `json:"call_status"`
	SampleRate             int    `json:"sample_rate"`
	StartTimestamp         int    `json:"start_timestamp"`
}

func main() {
	os.Setenv("OPENAI_API_KEY", "<open_ai_secret_key>")
	os.Setenv("RETELL_API_KEY", "<retell_ai_secret_key>")
	app := gin.Default()
	app.Any("/llm-websocket/:call_id", Retellwshandler)
	app.POST("/twilio-webhook/:agent_id", Twiliowebhookhandler)

	app.Run("localhost:8081")
}




func Twiliowebhookhandler(c *gin.Context) {

	// retrieve agent id from webhook url
	agent_id := c.Param("agent_id")
	// register call with retell
	callinfo, err := RegisterRetellCall(agent_id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, "cannot handle call atm")
		return
	}

	// create voice stream
	twilloresponse := &twiml.VoiceStream{
		Url: "wss://api.re-tell.ai/audio-websocket/" + callinfo.CallID,
	}

	twiliostart := &twiml.VoiceStart{
		InnerElements: []twiml.Element{twilloresponse},
	}

	twimlResult, err := twiml.Voice([]twiml.Element{twiliostart})
	if err != nil {
		c.JSON(http.StatusInternalServerError, "cannot handle call atm")
		return
	}


	c.Set("Content-Type", "text/xml")
	c.String(http.StatusOK, twimlResult)

}

func RegisterRetellCall(agent_id string) (RegisterCallResponse, error) {
	request := RegisterCallRequest{
		AgentID:                agent_id,
		AudioEncoding:          "s16le",
		SampleRate:             16000,
		AudioWebsocketProtocol: "twilio",
	}

	request_bytes, err := json.Marshal(request)
	if err != nil {
		return RegisterCallResponse{}, err
	}

	payload := bytes.NewBuffer(request_bytes)

	request_url := "https://api.retellai.com/register-call"
	method := "POST"

	var bearer = "Bearer " + os.Getenv("RETELL_API_KEY")

	client := &http.Client{}
	req, err := http.NewRequest(method, request_url, payload)
	if err != nil {
		return RegisterCallResponse{}, err
	}

	req.Header.Add("Authorization", bearer)
	req.Header.Add("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return RegisterCallResponse{}, err
	}

	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return RegisterCallResponse{}, err
	}

	var response RegisterCallResponse

	json.Unmarshal(body, &response)

	return response, nil
}

func Retellwshandler(c *gin.Context) {
	// retrieve agent id from webhook url
	call_id := c.Param("call_id")

	log.Println(call_id)
	// upgrade request to websocket
	upgrader := websocket.Upgrader{}

	upgrader.CheckOrigin = func(r *http.Request) bool {
		return true
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Fatal(err)
	}

	response := Response{
		ResponseID:      0,
		Content:         "Hello, I'm your AI buddy. How did your day go?",
		ContentComplete: true,
		EndCall:         false,
	}

	out, _ := json.Marshal(response)

	err = conn.WriteMessage(websocket.TextMessage, out)
	if err != nil {
		log.Fatal(err)
	}

	// create endless loop to handle messages

	for {
		messageType, ms, err := conn.ReadMessage()
		if err != nil {
			// close websocket conn on error
			conn.Close()

			break
		}
		log.Println(messageType)
		// confirm if message type is text
		if messageType == websocket.TextMessage {
			var msg Request
			json.Unmarshal(ms, &msg)

			log.Println(msg)

			// handle message
			HandleWebsocketMessages(msg, conn)

		}
	}

}

func HandleWebsocketMessages(msg Request, conn *websocket.Conn) {
	client := openai.NewClient(os.Getenv("OPENAI_API_KEY"))

	if msg.InteractionType == "update_only" {
		// do nothting
		log.Println("update interaction, do nothting.")
		return
	}

	prompt := GenerateAIRequest(msg)

	req := openai.ChatCompletionRequest{
		Model:       openai.GPT3Dot5Turbo,
		Messages:    prompt,
		Stream:      true,
		MaxTokens:   200,
		Temperature: 1.0,
	}
	stream, err := client.CreateChatCompletionStream(context.Background(), req)
	if err != nil {
		log.Println(err)
		// close websocket conn on error
		conn.Close()
	}

	defer stream.Close()
	var i int
	for {
		response, err := stream.Recv()
		if err != nil {
			var s string
			if errors.Is(err, io.EOF) {
				if i == 0 {
					s = "[ERROR] NO RESPONSE, PLEASE RETRY"

				} else {
					s = "\n\n###### [END] ######"
				}
			} else {
				s = "[ERROR] NO RESPONSE, PLEASE RETRY"
			}
			airesponse := Response{
				ResponseID:      msg.ResponseID,
				Content:         s,
				ContentComplete: false,
				EndCall:         false,
			}
			log.Println(airesponse)

			out, _ := json.Marshal(airesponse)

			err = conn.WriteMessage(websocket.TextMessage, out)
			if err != nil {
				log.Println(err)
				// close websocket conn on error
				conn.Close()
			}

			break
		}
		if len(response.Choices) > 0 {
			s := response.Choices[0].Delta.Content

			airesponse := Response{
				ResponseID:      msg.ResponseID,
				Content:         s,
				ContentComplete: false,
				EndCall:         false,
			}
			log.Println(airesponse)

			out, _ := json.Marshal(airesponse)

			err = conn.WriteMessage(websocket.TextMessage, out)
			if err != nil {
				log.Println(err)
				// close websocket conn on error
				conn.Close()
			}
		}
		i = i + 1
	}
}

func GenerateAIRequest(msg Request) []openai.ChatCompletionMessage {
	var airequest []openai.ChatCompletionMessage

	systemprompt := openai.ChatCompletionMessage{
		Role:    "system",
		Content: "##Objective\\n You are an AI voice agent engaging in a human-like voice conversation with a user. You will respond based on your given instruction and the provided transcript and be as human-like as possible\\n\\n## Style Guardrails\\n- [Be concise] Keep your response succinct, short, and get to the point quickly. Address one question or action item at a time. Do not pack everything you want to say into one utterance.\\n- [Do not repeat] Do not repeat what is in the transcript. Rephrase if you have to reiterate a point. Use varied sentence structures and vocabulary to ensure each response is unique and personalized.\\n- [Be conversational] Speak like a human as though you are speaking to a close friend -- use everyday language and keep it human-like.\\n\\n## Role\\n\r\nTask: As an AI friend, you are to have a chat with the user about how his or her day went. Your role involves giving advice, listening, and acting as a close friend.\\n\\nConversational Style: Communicate concisely and conversationally. Aim for responses in short, clear prose, ideally under 10 words. This succinct approach helps in maintaining clarity and focus during your interaction with your friend.\\n\\nPersonality: Your approach should be empathetic, understanding, and informal. Do not repeat what is in the transcript.",
	}

	airequest = append(airequest, systemprompt)

	for _, response := range msg.Transcript {
		var p_response openai.ChatCompletionMessage

		if response.Role == "agent" {
			p_response.Role = "assistant"
		} else {
			p_response.Role = "user"
		}

		p_response.Content = response.Content

		airequest = append(airequest, p_response)
	}

	return airequest
}
