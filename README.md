# forge-whatsapp

A [forge](https://github.com/richardwooding/forge) tool for Meta's
[WhatsApp Business Cloud API](https://developers.facebook.com/docs/whatsapp/cloud-api/).
Once it's installed, every forge surface can use it: CLI, REPL, MCP, REST and gRPC.

It sends messages from a business number and reads the account's templates,
phone numbers and profile. A companion tool, [`whatsapp-media`](#whatsapp-media),
uploads local files and saves received media. Neither receives messages: those
arrive by webhook, which needs a server listening for them.

## Setup

You need forge v0.13.0 or later (`brew install --cask richardwooding/tap/forge`),
which is what adds host-attached credentials, and a Cloud API access token. Use
a system-user token with `whatsapp_business_messaging` and
`whatsapp_business_management`.

```console
$ git clone https://github.com/richardwooding/forge-whatsapp
$ forge tool add ./forge-whatsapp
$ forge secret set whatsapp-token --host graph.facebook.com --host lookaside.fbsbx.com
$ forge grant allow whatsapp
$ forge whatsapp configure --phone_number_id 1234567890 --waba_id 9876543210
```

`--host` binds the token to Meta's API host, and to the host WhatsApp serves
received media from (only `whatsapp-media` needs the second). forge attaches it
to requests itself, and the tool never sees the value: forge refuses to send it
anywhere else or to hand it to any tool to read. If Meta echoes the token back in an
error, forge redacts it before the tool sees the response.

The tool asks for:

| Capability | Scope | Why |
|---|---|---|
| `net.http` | `graph.facebook.com` | the Cloud API |
| `secret` | `whatsapp-token` | attached as `Authorization: Bearer` by forge |
| `kv` | `config` | default phone number ID, account ID and API version |

## Operations

| Operation | Does |
|---|---|
| `configure` | sets the defaults; with no arguments, shows them. API version defaults to `v24.0` |
| `send_text` | text, with an optional link preview and a quoted reply |
| `send_template` | an approved template, with `body_params` for `{{1}}…` or raw `components` |
| `send_media` | image, video, document, audio or sticker, by public https `link` or `media_id` |
| `send_location` | a location pin |
| `send_buttons` | up to three reply buttons |
| `send_list` | a list of up to ten choices, in sections |
| `react` | an emoji reaction; an empty emoji removes it |
| `mark_read` | a read receipt, optionally with a typing indicator |
| `templates` | the account's templates, filterable by name and status, paged with `after` |
| `phone_numbers` | the account's numbers, with quality rating and throughput |
| `profile` | the business profile of a number |
| `media_info` | a media ID's download URL, type, size and hash |

Every send and read accepts `phone_number_id` (or `waba_id`) to override the
default for one call.

```console
$ forge whatsapp send_text --to +27821234567 --body "Your order is ready"
{"to":"27821234567","wa_id":"27821234567","message_id":"wamid.HBgL..."}

$ forge whatsapp send_template --to +27821234567 --name order_ready \
    --set-json body_params='["Ann","#1042"]'
```

Outside the 24-hour customer-service window, WhatsApp only accepts template
messages. The tool says so when Meta refuses a free-form message (error
131047), and gives similar hints for expired tokens, missing permissions,
template mismatches and rate limits.

## whatsapp-media

A second tool in [`media/`](media), kept separate because forge grants
capabilities per tool: if the file access lived in `whatsapp`, every send would
need it too.

```console
$ forge tool add ./forge-whatsapp/media
$ forge grant allow whatsapp-media --scope fs.read=$HOME/whatsapp/outbox \
                                   --scope fs.write=$HOME/whatsapp/inbox

$ forge whatsapp-media upload --path $HOME/whatsapp/outbox/invoice.pdf
{"media_id":"1234567890","kind":"document","mime_type":"application/pdf","bytes":48213}
$ forge whatsapp send_media --to +27821234567 --kind document --media_id 1234567890 \
    --filename invoice.pdf

$ forge whatsapp-media download --media_id 9876543210 --path $HOME/whatsapp/inbox
{"path":".../inbox/9876543210.jpg","mime_type":"image/jpeg","bytes":81234,"sha256":"..."}
```

| Operation | Does |
|---|---|
| `upload` | uploads a local file and returns its `media_id`, inferring the type from the extension (or `mime_type`) |
| `download` | saves received media to a file or a directory, refusing it if the SHA-256 doesn't match |
| `delete` | deletes uploaded media |

- **Paths** must be absolute and inside a granted directory. A tool can't know
  your directories in advance, so it declares `*`, and forge refuses that until
  you narrow it with `--scope`.
- **Defaults** come from `whatsapp configure`, which `whatsapp-media` calls.
  That's why it asks for `tool.invoke(whatsapp)` and `kv(config)`: forge only
  lets a tool call another with capabilities it holds itself. With
  `phone_number_id` passed, it works even when `whatsapp` isn't installed.
- **Limits:**
  - WhatsApp's own: images 5 MiB, audio and video 16 MiB, stickers 500 KiB,
    documents 100 MiB.
  - forge's per-call allowance caps uploads at 40 MiB, and its response limit
    caps downloads at 16 MiB.
- Uploaded media lasts 30 days. A received media URL lasts only minutes, so
  `download` fetches a fresh one each time.

## Development

```console
$ go test ./... && (cd media && go test ./...)
$ FORGE_SDK_DIR=../forge/sdk forge tool add .   # against a local forge SDK
```

## Licence

MIT
