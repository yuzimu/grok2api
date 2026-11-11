package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/chenyme/grok2api/backend/internal/domain/account"
	mediadomain "github.com/chenyme/grok2api/backend/internal/domain/media"
	infraegress "github.com/chenyme/grok2api/backend/internal/infra/egress"
	"github.com/chenyme/grok2api/backend/internal/infra/provider"
)

const buildImageModel = "grok-imagine-image-2.0"

// SetImageAssetStore 装配 Web 与 Build 共用的图片存储，在注册适配器前调用。
func (a *Adapter) SetImageAssetStore(store provider.ImageAssetStore) {
	a.imageAssets = store
}

// GenerateImage 使用 Build OAuth 向主地址提交一次同步文生图请求。
func (a *Adapter) GenerateImage(ctx context.Context, input provider.ImageGenerationRequest) (*provider.Response, error) {
	payload, err := buildImagePayload(input)
	if err != nil {
		return invalidResponsesResponse(err), nil
	}
	return a.doBuildImage(ctx, input.Credential, input.Model, "/images/generations", payload)
}

// EditImage 按官方协议发送参考图，并以网关图片资产格式交付结果。
func (a *Adapter) EditImage(ctx context.Context, input provider.ImageEditRequest) (*provider.Response, error) {
	if input.Count < 0 || input.Count > 1 {
		return invalidResponsesResponse(errors.New("Build 图片编辑仅支持 n=1")), nil
	}
	if len(input.SelectionRegions) != 0 || len(input.RegionEdits) != 0 {
		return invalidResponsesResponse(errors.New("Build 图片编辑不支持选区或分段编辑")), nil
	}
	if len(input.ImageURLs) == 0 || len(input.ImageURLs) > mediadomain.ReferenceImageLimit(input.Model) {
		return invalidResponsesResponse(errors.New("Build 图片编辑参考图数量无效")), nil
	}
	payload, err := buildImagePayload(provider.ImageGenerationRequest{
		Model: input.Model, Prompt: input.Prompt, Count: input.Count, Size: input.Size,
		AspectRatio: input.AspectRatio, Resolution: input.Resolution, Quality: input.Quality,
		ResponseFormat: input.ResponseFormat, Streaming: input.Streaming, PartialImages: input.PartialImages,
	})
	if err != nil {
		return invalidResponsesResponse(err), nil
	}
	images := make([]map[string]string, 0, len(input.ImageURLs))
	for _, raw := range input.ImageURLs {
		value := strings.TrimSpace(raw)
		if !validBuildEditImage(value) {
			return invalidResponsesResponse(errors.New("参考图必须是 HTTPS URL 或 image data URL")), nil
		}
		images = append(images, map[string]string{"type": "image_url", "url": value})
	}
	if len(images) == 1 {
		payload["image"] = images[0]
	} else {
		payload["images"] = images
	}
	if a.imageAssets == nil {
		return nil, provider.NewMediaPostProcessingError(provider.MediaPostProcessingStorage, errors.New("图片媒体存储未配置"))
	}
	format := payload["response_format"].(string)
	// 直接接收图片字节，避免从任意上游 URL 回下载或转发凭据。
	payload["response_format"] = "b64_json"
	resp, err := a.doBuildImage(ctx, input.Credential, input.Model, "/images/edits", payload)
	if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp, err
	}
	return a.saveBuildImageEdit(ctx, resp, format)
}

// validBuildEditImage 接受上游可读取的 URL 或内联图片，拒绝用户信息与其他协议。
func validBuildEditImage(value string) bool {
	if strings.HasPrefix(strings.ToLower(value), "data:image/") {
		meta, data, ok := strings.Cut(value, ",")
		return ok && strings.HasSuffix(strings.ToLower(meta), ";base64") && data != ""
	}
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "https" && parsed.Hostname() != "" && parsed.User == nil
}

// saveBuildImageEdit 保存原编辑结果，并统一返回本地 URL 或 Base64。
func (a *Adapter) saveBuildImageEdit(ctx context.Context, resp *provider.Response, format string) (*provider.Response, error) {
	defer resp.Body.Close()
	const maxEditResponseBytes = 32 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxEditResponseBytes+1))
	if err != nil || len(body) > maxEditResponseBytes {
		return nil, provider.NewMediaPostProcessingError(provider.MediaPostProcessingDownload, errors.New("Build 编辑响应读取失败或超出限制"))
	}
	var decoded struct {
		Created int64           `json:"created"`
		Usage   json.RawMessage `json:"usage"`
		Data    []struct {
			Base64        string `json:"b64_json"`
			RevisedPrompt string `json:"revised_prompt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil || len(decoded.Data) != 1 {
		return nil, provider.NewMediaPostProcessingError(provider.MediaPostProcessingDownload, errors.New("Build 编辑未返回单张图片"))
	}
	source := decoded.Data[0]
	raw, err := base64.StdEncoding.DecodeString(source.Base64)
	if err != nil || len(raw) == 0 {
		return nil, provider.NewMediaPostProcessingError(provider.MediaPostProcessingDownload, errors.New("Build 编辑图片 Base64 无效"))
	}
	asset, err := a.imageAssets.SaveImage(ctx, raw)
	if err != nil {
		return nil, provider.NewMediaPostProcessingError(provider.MediaPostProcessingStorage, err)
	}
	item := map[string]string{"mime_type": asset.MIMEType, "revised_prompt": source.RevisedPrompt}
	if format == "b64_json" {
		item["b64_json"] = source.Base64
	} else {
		item["url"] = a.imageAssets.PublicImageURL(asset.ID)
	}
	created := decoded.Created
	if created == 0 {
		created = time.Now().Unix()
	}
	result := map[string]any{"created": created, "data": []map[string]string{item}}
	if len(decoded.Usage) != 0 {
		result["usage"] = decoded.Usage
	}
	output, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("编码 Build 编辑结果: %w", err)
	}
	resp.Body = io.NopCloser(bytes.NewReader(output))
	resp.Header = http.Header{"Content-Type": []string{"application/json"}}
	return resp, nil
}

// doBuildImage 统一使用 Build 主地址及现有 OAuth、出口配置。
func (a *Adapter) doBuildImage(ctx context.Context, credential account.Credential, model, path string, payload map[string]any) (*provider.Response, error) {
	token, err := a.cipher.Decrypt(credential.EncryptedAccessToken)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	endpoint := a.urlWithBase(a.primaryBaseURL(), path)
	req, err := http.NewRequestWithContext(infraegress.WithCredential(ctx, credential), http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if err := a.applyHeaders(req, credential, token, model, "", true); err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.http.Do(req)
	if err != nil {
		return nil, err
	}
	if err := normalizeGzipResponse(resp); err != nil {
		return nil, err
	}
	return &provider.Response{
		StatusCode: resp.StatusCode, Status: resp.Status, Header: resp.Header,
		Body: resp.Body, UpstreamURL: endpoint,
	}, nil
}

// buildImagePayload 仅发送官方图片字段，将网关的 size 转为 aspect_ratio。
func buildImagePayload(input provider.ImageGenerationRequest) (map[string]any, error) {
	if input.Model != buildImageModel {
		return nil, errors.New("模型不支持 Build 图片生成")
	}
	if input.Streaming || input.PartialImages != 0 {
		return nil, errors.New("Build 图片生成不支持 stream 或 partial_images")
	}
	prompt := strings.TrimSpace(input.Prompt)
	if prompt == "" {
		return nil, errors.New("prompt 不能为空")
	}
	if input.Count < 0 || input.Count > 10 {
		return nil, errors.New("n 必须在 1 到 10 之间")
	}
	format := strings.ToLower(strings.TrimSpace(input.ResponseFormat))
	switch format {
	case "":
		format = "url"
	case "url", "b64_json":
	default:
		return nil, errors.New("response_format 必须是 url 或 b64_json")
	}
	payload := map[string]any{"model": input.Model, "prompt": prompt, "n": max(1, input.Count), "response_format": format}
	ratio := strings.ToLower(strings.TrimSpace(input.AspectRatio))
	if ratio == "" {
		size := strings.ToLower(strings.TrimSpace(input.Size))
		switch size {
		case "1024x1024":
			ratio = "1:1"
		case "1280x720":
			ratio = "16:9"
		case "720x1280":
			ratio = "9:16"
		case "1792x1024", "1536x1024":
			ratio = "3:2"
		case "1024x1792", "1024x1536":
			ratio = "2:3"
		default:
			ratio = size
		}
	}
	switch ratio {
	case "", "auto":
	case "1:1", "16:9", "9:16", "4:3", "3:4", "3:2", "2:3", "2:1", "1:2":
		payload["aspect_ratio"] = ratio
	default:
		return nil, errors.New("aspect_ratio 或 size 不受支持")
	}
	if resolution := strings.ToLower(strings.TrimSpace(input.Resolution)); resolution != "" {
		if resolution != "1k" && resolution != "2k" {
			return nil, errors.New("resolution 必须是 1k 或 2k")
		}
		payload["resolution"] = resolution
	}
	if quality := strings.ToLower(strings.TrimSpace(input.Quality)); quality != "" {
		if quality != "low" && quality != "medium" {
			return nil, errors.New("quality 必须是 low 或 medium")
		}
		payload["quality"] = quality
	}
	return payload, nil
}
