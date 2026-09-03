package captcha

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/wenlng/go-captcha-assets/resources/images"
	"github.com/wenlng/go-captcha-assets/resources/tiles"
	"github.com/wenlng/go-captcha/v2/base/option"
	"github.com/wenlng/go-captcha/v2/slide"
)

const (
	SceneLogin    = "login"
	SceneRegister = "register"
	ImageWidth    = 300
	ImageHeight   = 220

	challengeTTL = 3 * time.Minute
	proofTTL     = 5 * time.Minute
)

var (
	ErrInvalidScene       = errors.New("invalid CAPTCHA scene")
	ErrInvalidChallenge   = errors.New("invalid or expired CAPTCHA challenge")
	ErrVerificationFailed = errors.New("CAPTCHA verification failed")

	slideCaptcha     slide.Captcha
	slideCaptchaErr  error
	slideCaptchaOnce sync.Once
)

type Position struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type Challenge struct {
	CaptchaKey string `json:"captcha_key"`
	Image      string `json:"image"`
	Tile       string `json:"tile"`
	TileX      int    `json:"tile_x"`
	TileY      int    `json:"tile_y"`
	TileWidth  int    `json:"tile_width"`
	TileHeight int    `json:"tile_height"`
	ExpiresAt  int64  `json:"expires_at"`
}

type challengePayload struct {
	X int `json:"x"`
	Y int `json:"y"`
}

func IsValidScene(scene string) bool {
	return scene == SceneLogin || scene == SceneRegister
}

func IsInvalidTokenError(err error) bool {
	return errors.Is(err, ErrInvalidChallenge) ||
		errors.Is(err, model.ErrAuthFlowInvalid) ||
		errors.Is(err, model.ErrAuthFlowExpired) ||
		errors.Is(err, model.ErrAuthFlowConsumed)
}

func getSlideCaptcha() (slide.Captcha, error) {
	slideCaptchaOnce.Do(func() {
		tileImages, err := tiles.GetTiles()
		if err != nil {
			slideCaptchaErr = fmt.Errorf("load CAPTCHA tiles: %w", err)
			return
		}
		graphs := make([]*slide.GraphImage, 0, len(tileImages))
		for _, tileImage := range tileImages {
			if tileImage == nil {
				continue
			}
			graphs = append(graphs, &slide.GraphImage{
				OverlayImage: tileImage.OverlayImage,
				ShadowImage:  tileImage.ShadowImage,
				MaskImage:    tileImage.MaskImage,
			})
		}
		if len(graphs) == 0 {
			slideCaptchaErr = errors.New("no CAPTCHA tile resources loaded")
			return
		}
		backgrounds, err := images.GetImages()
		if err != nil {
			slideCaptchaErr = fmt.Errorf("load CAPTCHA backgrounds: %w", err)
			return
		}

		builder := slide.NewBuilder(
			slide.WithImageSize(option.Size{Width: ImageWidth, Height: ImageHeight}),
			slide.WithRangeGraphSize(option.RangeVal{Min: 58, Max: 66}),
		)
		builder.SetResources(
			slide.WithGraphImages(graphs),
			slide.WithBackgrounds(backgrounds),
		)
		slideCaptcha = builder.Make()
	})
	return slideCaptcha, slideCaptchaErr
}

func Generate(scene string) (*Challenge, error) {
	if !IsValidScene(scene) {
		return nil, ErrInvalidScene
	}
	capt, err := getSlideCaptcha()
	if err != nil {
		return nil, err
	}
	data, err := capt.Generate()
	if err != nil {
		return nil, fmt.Errorf("generate CAPTCHA: %w", err)
	}
	block := data.GetData()
	if block == nil {
		return nil, errors.New("generated CAPTCHA has no puzzle block")
	}
	master, err := data.GetMasterImage().ToBase64()
	if err != nil {
		return nil, fmt.Errorf("encode CAPTCHA image: %w", err)
	}
	tile, err := data.GetTileImage().ToBase64()
	if err != nil {
		return nil, fmt.Errorf("encode CAPTCHA tile: %w", err)
	}
	payload, err := common.Marshal(challengePayload{X: block.X, Y: block.Y})
	if err != nil {
		return nil, fmt.Errorf("marshal CAPTCHA challenge: %w", err)
	}
	expiresAt := time.Now().Add(challengeTTL)
	key, _, err := model.CreateAuthFlow(model.AuthFlowCreate{
		Purpose:   model.AuthFlowPurposeCaptchaChallenge,
		Provider:  scene,
		Payload:   string(payload),
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return nil, fmt.Errorf("store CAPTCHA challenge: %w", err)
	}
	return &Challenge{
		CaptchaKey: key,
		Image:      master,
		Tile:       tile,
		TileX:      block.DX,
		TileY:      block.Y,
		TileWidth:  block.Width,
		TileHeight: block.Height,
		ExpiresAt:  expiresAt.Unix(),
	}, nil
}

func Verify(scene, key string, position Position) (string, error) {
	if !IsValidScene(scene) {
		return "", ErrInvalidScene
	}
	flow, err := model.ConsumeAuthFlow(key, model.AuthFlowMatch{
		Purpose:  model.AuthFlowPurposeCaptchaChallenge,
		Provider: scene,
	})
	if err != nil {
		if IsInvalidTokenError(err) {
			return "", ErrInvalidChallenge
		}
		return "", err
	}
	defer deleteFlow(flow)

	var payload challengePayload
	if err := common.UnmarshalJsonStr(flow.Payload, &payload); err != nil {
		return "", ErrInvalidChallenge
	}
	if !slide.Validate(position.X, position.Y, payload.X, payload.Y, 5) {
		return "", ErrVerificationFailed
	}

	proof, _, err := model.CreateAuthFlow(model.AuthFlowCreate{
		Purpose:   model.AuthFlowPurposeCaptchaProof,
		Provider:  scene,
		ExpiresAt: time.Now().Add(proofTTL),
	})
	if err != nil {
		return "", fmt.Errorf("store CAPTCHA proof: %w", err)
	}
	return proof, nil
}

func ConsumeProof(scene, token string) error {
	if !IsValidScene(scene) {
		return ErrInvalidScene
	}
	flow, err := model.ConsumeAuthFlow(token, model.AuthFlowMatch{
		Purpose:  model.AuthFlowPurposeCaptchaProof,
		Provider: scene,
	})
	if err != nil {
		return err
	}
	deleteFlow(flow)
	return nil
}

func deleteFlow(flow *model.AuthFlow) {
	if flow == nil {
		return
	}
	if err := model.DeleteAuthFlow(flow.Id); err != nil {
		common.SysError(fmt.Sprintf("failed to delete consumed CAPTCHA flow %d: %v", flow.Id, err))
	}
}
