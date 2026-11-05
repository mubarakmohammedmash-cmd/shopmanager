# Backend configuration

## AI assistant

The in-app assistant uses the OpenAI API for natural-language understanding and tool selection. Configure these environment variables on the backend:

- `OPENAI_API_KEY` — required. Keep this secret on the server; never put it in a frontend environment variable.
- `OPENAI_MODEL` — optional. Defaults to `gpt-4o-mini`.

The assistant sends the user's recent chat messages and only the store data needed for an enabled read tool (today's summary, matching products, low-stock products, or recent sales) to OpenAI. Requests are authenticated and store-scoped. Sales representatives only receive their own sales and do not receive product costs or profit figures. Voice input is transcribed by the browser's speech-recognition service; only the resulting text is sent to this backend.

The assistant can navigate to screens available to the signed-in role. It can prepare a product draft, but it does not save it: a manager must review the details and explicitly choose **Confirm and add**. Other writes, including sales, payments, and voids, are not available through the assistant.

For local development, set `OPENAI_API_KEY` in the backend process environment before starting the Go server. For deployment, configure it in the backend service's environment settings. API usage is billed separately from a ChatGPT subscription.
