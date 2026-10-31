package telemetry

import (
	"strconv"
	"time"

	"github.com/NARUBROWN/spine/core"
)

type TimingInterceptor struct{}

func NewTimingInterceptor() *TimingInterceptor { return &TimingInterceptor{} }

func (*TimingInterceptor) PreHandle(ctx core.ExecutionContext, _ core.HandlerMeta) error {
	ctx.Set("tech-blog.request-start", time.Now())
	return nil
}

func (*TimingInterceptor) PostHandle(core.ExecutionContext, core.HandlerMeta) {}

func (*TimingInterceptor) BeforeResponse(ctx core.ExecutionContext, _ core.HandlerMeta, _ error) error {
	value, _ := ctx.Get("tech-blog.request-start")
	start, ok := value.(time.Time)
	if !ok {
		return nil
	}
	value, _ = ctx.Get("spine.response_writer")
	writer, ok := value.(core.ResponseWriter)
	if ok {
		// 인증, DB 조회와 성공 응답 직렬화까지 포함하며 네트워크 전송은 제외합니다.
		ms := float64(time.Since(start).Microseconds()) / 1000
		writer.AddHeader("Server-Timing", "app;dur="+strconv.FormatFloat(ms, 'f', 3, 64))
	}
	return nil
}

func (*TimingInterceptor) AfterCompletion(core.ExecutionContext, core.HandlerMeta, error) {}
