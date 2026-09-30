# forge-whatsapp

A [forge](https://github.com/richardwooding/forge) tool for Meta's
[WhatsApp Business Cloud API](https://developers.facebook.com/docs/whatsapp/cloud-api/).
Once it's installed, every forge surface can use it: CLI, REPL, MCP, REST and gRPC.

It sends messages from a business number and reads the account's templates,
phone numbers and profile. It does not receive messages: those arrive by
webhook, which needs a server listening for them.

## Setup

You need forge v0.13.0 or later (`brew install --cask richardwooding/tap/forge`),
which is what adds host-attached credentials, and a Cloud API access token. Use a system-user token with
`whatsapp_business_messaging` and `whatsapp_business_management`.

```console
$ git clone https://github.com/richardwooding/forge-whatsapp
$ forge tool add ./forge-whatsapp
$ forge secret set whatsapp-token --host graph.facebook.com  # prompts, no echo
$ forge grant allow whatsapp
$ forge whatsapp configure --phone_number_id 1234567890 --waba_id 9876543210
```

`--host` binds the token to `graph.facebook.com`. forge attaches it to requests
itself, and the tool never sees the value: forge refuses to send it anywhere
else or to hand it to any tool to read. If Meta echoes the token back in an
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

## Development

```console
$ go test ./...
$ FORGE_SDK_DIR=../forge/sdk forge tool add .   # against a local forge SDK
```

## Licence

MIT
