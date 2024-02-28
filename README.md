# Build an AI Buddy with Go, OpenAI, Retell AI, and Twilio Programmable Voice 

The code lets you run an AI buddy with Go, Open AI, Retell AI, and Twilio Programmable Voice.

### Prerequisites

To run this code, you need:

- A [Twilio account and Twilio phone number](https://www.twilio.com/try-twilio)
- An [OpenAI account](https://openai.com/)
- A [Retell AI account](https://www.retellai.com/)
- [Go](https://go.dev/doc/install) installed on your local machine
- [The ngrok CLI (or alternative tunnel service)](https://ngrok.com/)

### Run the application

To run the code,
1. Change `<enter_openai_secret_key>` to your Open AI secret key and change `<enter_retell_ai_secret_key>` to your Retell AI secret key in the `main.go` file.
2. Start your Go server. 
   ```
   go run main.go
   ```
3. Generate an HTTPS URL for your server with ngrok.
   ```
   ngrok http 8081
   ```
4. Create a Retell AI agent with the WebSocket URL.
5. Add your webhook URL to your Twilio phone number.