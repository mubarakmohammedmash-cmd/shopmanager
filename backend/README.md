# Backend configuration

## Persistent database on Render

The backend stores its SQLite database in the file configured by `DB_PATH`. Render's service filesystem is temporary, so the database must be on a persistent disk. In the existing backend web service settings:

1. Add a persistent disk mounted at `/var/data`.
2. Set `DB_PATH` to `/var/data/shop.db`.
3. Apply the disk and environment changes, then deploy/restart the service.

The server now refuses to start on Render if `DB_PATH` is missing or relative. This prevents a healthy-looking deployment from silently creating an empty database on temporary storage. For local development, the default remains `shop.db`.

The current live backend reports that setup is needed, which means its running database contains no users. Persistence prevents future resets but does not restore accounts or data that are already missing. Restore a backup to `/var/data/shop.db` before restarting if one is available; otherwise, configure the disk first and create the manager account again only after the backend starts with the persistent path.

## AI assistant

The in-app assistant uses OpenAI for general questions, image and document analysis, optional web search, and shop-specific tools. Configure these environment variables on the backend:

- `OPENAI_API_KEY` — required. Keep this secret on the server; never put it in a frontend environment variable.
- `OPENAI_MODEL` — optional. Defaults to `gpt-4o-mini`.
- `OPENAI_RESPONSES_MODEL` — optional. Model for image/document analysis and web search; defaults to `OPENAI_MODEL`, then `gpt-4o-mini`.

The assistant sends recent chat messages to OpenAI. Image and document attachments (up to 3 files, 4 MB each) and explicitly requested web searches use the OpenAI Responses API. Supported attachments include common image formats, PDF, text, spreadsheets, and Office documents. Hands-free voice conversation uses the microphone and OpenAI transcription API: pauses automatically send each turn, and the assistant listens again after speaking its reply with browser speech synthesis. Microphone access requires a secure browser context (HTTPS or localhost). Web-search citations are shown as links in the assistant.

Requests are authenticated and store-scoped. Shop data is provided only when the assistant calls an enabled read tool. Sales representatives only receive their own sales and do not receive product costs or profit figures. The assistant can create a daily sales chart from permitted sales data, navigate to screens available to the signed-in role, and prepare a product draft. It does not save the draft: a manager must review and explicitly choose **Confirm and add**. Other writes, including sales, payments, and voids, are not available through the assistant.

Uploaded files, messages, and web searches are sent to OpenAI for processing. OpenAI API usage may be billed separately from a ChatGPT subscription. For local development, set `OPENAI_API_KEY` in the backend process environment before starting the Go server. For deployment, configure it in the backend service's environment settings.

## Super admin

The global user-management dashboard is available only to the existing account whose username is configured in the backend's `SUPER_ADMIN_USERNAME` environment variable. For this deployment, set `SUPER_ADMIN_USERNAME=Mubarak` in the backend service settings, then have that account sign out and back in. Username matching is case-insensitive. The configured username is reserved and cannot be claimed through store signup or staff creation.

The super admin can view users across all stores and disable or reactivate accounts. Disabling an account revokes its active sessions but preserves its sales history. The configured super-admin account cannot be disabled from the dashboard. The role is granted by backend configuration, not by client-supplied account data.
