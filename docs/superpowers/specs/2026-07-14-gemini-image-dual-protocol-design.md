# Gemini Image Dual-Protocol Design

## Goal

Make every model whose normalized name starts with `gemini` and contains
`image` available through both of these client-facing request modes by default:

- OpenAI image generation: `POST /v1/images/generations`
- Gemini native image generation: `POST /v1beta/models/{model}:generateContent`
  and the existing `/v1/models/{model}:generateContent` alias

The gateway keeps a request native when the selected channel supports its
source mode. When the selected channel only supports the other mode, the
gateway converts the request and converts the response back to the client's
source format.

## Scope

The automatic behavior applies only when the model name, after trimming and
lowercasing, satisfies both conditions:

1. starts with `gemini`;
2. contains `image`.

Imagen models such as `imagen-4.0-generate-001` remain on the existing Imagen
`predict` path. Other multimodal Gemini models that do not contain `image` are
unchanged.

This change covers synchronous and the existing local asynchronous wrapper for
`/v1/images/generations?async=true`. It does not add a new public route or a new
task model.

## Approaches Considered

### 1. Add a symmetric global conversion graph

Add `openai.image.generations -> gemini.generate_content` beside the existing
reverse conversion. This represents the protocol relationship explicitly, but
the current conversion graph treats channels without protocol declarations as
supporting every native mode. Changing that default would affect unrelated
channels and make this feature broader than necessary.

### 2. Extend the Gemini and Vertex provider adaptors

Keep `dto.ImageRequest` as the canonical request inside `ImageHelper`. For
Gemini image models, the Gemini adaptor converts it to `GeminiChatRequest`,
sends it to `generateContent`, and normalizes the Gemini response into
`dto.ImageResponse`. Vertex reuses the same conversion and response helpers.
Channel filtering treats OpenAI image generation and Gemini generateContent as
equivalent capabilities only for matching Gemini image models.

This is the selected approach. It follows the existing provider-adaptor
boundary already used to convert OpenAI image requests into Imagen `predict`
requests, while preserving native OpenAI image channels.

### 3. Add a default configurable protocol profile

A profile can already translate OpenAI image requests into a Gemini-native
provider contract. However, it requires `ChannelTypeConfigurable` and an
explicit `profile_id`, so it does not meet the requirement that ordinary Gemini
and Vertex channels support both request forms by default.

## Request Flow

### OpenAI image request on an OpenAI-image channel

The request remains `dto.ImageRequest` and follows the existing image adaptor.
No Gemini conversion occurs.

### OpenAI image request on a Gemini or Vertex channel

For a matching Gemini image model:

1. Preserve `model`, `prompt`, image count, requested dimensions, quality, and
   supported reference images from `dto.ImageRequest`.
2. Build a Gemini `contents` request with a user text part and optional image
   parts.
3. Force `generationConfig.responseModalities` to include `TEXT` and `IMAGE`,
   even for newly added Gemini image model names not yet present in the static
   `SupportedImagineModels` list.
4. Map supported size, aspect ratio, and image-size inputs into
   `generationConfig.imageConfig` using the same conventions as the existing
   OpenAI-chat-to-Gemini conversion.
5. Send the request to `{version}/models/{model}:generateContent`.
6. Parse returned Gemini candidate parts, collect image `inlineData`, and emit
   an OpenAI-compatible `dto.ImageResponse` with `b64_json` data.

If the upstream returns no image part, return a bad-upstream-response error
instead of an empty successful image response.

### Gemini generateContent request on a Gemini-native channel

Keep the existing behavior: the request and response remain Gemini native.

### Gemini generateContent request on an OpenAI-image channel

Keep the existing conversion:

1. Convert Gemini `contents`, candidate count, image config, and reference
   images into `dto.ImageRequest`.
2. Call the channel's OpenAI image generation mode.
3. Convert `dto.ImageResponse` back into Gemini candidate `inlineData` parts.

## Channel Selection

For matching Gemini image models, channel filtering accepts either:

- `gemini.generate_content`; or
- `openai.image.generations`.

The source mode remains preferred. A channel that explicitly declares the
source mode is not converted. A channel that declares only the other mode is
eligible because the gateway has a corresponding conversion path.

For all other models, existing `native_modes` and `enabled_conversions`
semantics remain unchanged.

## Error Handling

- Missing prompt remains an invalid image request.
- Invalid or unsupported reference image input returns a conversion error
  before the upstream call.
- Gemini HTTP and structured API errors continue through the existing relay
  error handling.
- A successful Gemini response without image data becomes a bad response, not
  an empty OpenAI image result.
- Imagen-only validation continues to apply only to `imagen-*` requests that
  use the Imagen `predict` request shape.

## Testing

Add focused regression coverage for:

1. the strict `gemini*image*` model predicate;
2. OpenAI image request conversion into Gemini `generateContent` payload;
3. size/image configuration and optional reference-image mapping;
4. Gemini response normalization into OpenAI image response;
5. Vertex reuse of the Gemini image conversion;
6. channel selection allowing either native mode for matching models;
7. preservation of the current source mode when it is native;
8. non-matching Gemini and Imagen models retaining existing behavior;
9. the existing Gemini-to-OpenAI-image conversion remaining green.

Verification will use focused Go tests for `common`, `relay/conversion`,
`relay/channel/gemini`, `relay/channel/vertex`, `relay/helper`, and `service`,
followed by `git diff --check`.
