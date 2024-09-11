// Code generated by templ - DO NOT EDIT.

// templ: version: v0.2.778
package pages

//lint:file-ignore SA4006 This context is only used if a nested component is present.

import "github.com/a-h/templ"
import templruntime "github.com/a-h/templ/runtime"

import (
	"github.com/ArkLabsHQ/ark-node/internal/interface/web/templates/components"
	"github.com/ArkLabsHQ/ark-node/internal/interface/web/types"
)

func HistoryIcon(kind, status string) templ.Component {
	if status == "pending" {
		return PendingIcon()
	}
	if status == "waiting" {
		return WaitingIcon()
	}
	switch kind {
	case "received":
		return ReceivedIcon()
	case "sent":
		return SentIcon()
	case "swap":
		return SwapIcon()
	default:
		return SwapIcon()
	}
}

func PendingIcon() templ.Component {
	return templruntime.GeneratedTemplate(func(templ_7745c5c3_Input templruntime.GeneratedComponentInput) (templ_7745c5c3_Err error) {
		templ_7745c5c3_W, ctx := templ_7745c5c3_Input.Writer, templ_7745c5c3_Input.Context
		if templ_7745c5c3_CtxErr := ctx.Err(); templ_7745c5c3_CtxErr != nil {
			return templ_7745c5c3_CtxErr
		}
		templ_7745c5c3_Buffer, templ_7745c5c3_IsBuffer := templruntime.GetBuffer(templ_7745c5c3_W)
		if !templ_7745c5c3_IsBuffer {
			defer func() {
				templ_7745c5c3_BufErr := templruntime.ReleaseBuffer(templ_7745c5c3_Buffer)
				if templ_7745c5c3_Err == nil {
					templ_7745c5c3_Err = templ_7745c5c3_BufErr
				}
			}()
		}
		ctx = templ.InitializeContext(ctx)
		templ_7745c5c3_Var1 := templ.GetChildren(ctx)
		if templ_7745c5c3_Var1 == nil {
			templ_7745c5c3_Var1 = templ.NopComponent
		}
		ctx = templ.ClearChildren(ctx)
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("<div class=\"flex items-center w-8 h-8 bg-greenbg/10 rounded-full mr-2\"><svg xmlns=\"http://www.w3.org/2000/svg\" class=\"w-4 h-4 mx-auto text-yellow\" viewBox=\"0 0 2048 2048\"><path fill=\"currentColor\" d=\"M1536 0v128H384V0h1152zm45 979l-90 90l-467-470v1449H896V599l-467 470l-90-90l621-626l621 626z\"></path></svg></div>")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		return templ_7745c5c3_Err
	})
}

func ReceivedIcon() templ.Component {
	return templruntime.GeneratedTemplate(func(templ_7745c5c3_Input templruntime.GeneratedComponentInput) (templ_7745c5c3_Err error) {
		templ_7745c5c3_W, ctx := templ_7745c5c3_Input.Writer, templ_7745c5c3_Input.Context
		if templ_7745c5c3_CtxErr := ctx.Err(); templ_7745c5c3_CtxErr != nil {
			return templ_7745c5c3_CtxErr
		}
		templ_7745c5c3_Buffer, templ_7745c5c3_IsBuffer := templruntime.GetBuffer(templ_7745c5c3_W)
		if !templ_7745c5c3_IsBuffer {
			defer func() {
				templ_7745c5c3_BufErr := templruntime.ReleaseBuffer(templ_7745c5c3_Buffer)
				if templ_7745c5c3_Err == nil {
					templ_7745c5c3_Err = templ_7745c5c3_BufErr
				}
			}()
		}
		ctx = templ.InitializeContext(ctx)
		templ_7745c5c3_Var2 := templ.GetChildren(ctx)
		if templ_7745c5c3_Var2 == nil {
			templ_7745c5c3_Var2 = templ.NopComponent
		}
		ctx = templ.ClearChildren(ctx)
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("<div class=\"flex items-center w-8 h-8 bg-greenbg/10 rounded-full mr-2\"><svg xmlns=\"http://www.w3.org/2000/svg\" class=\"w-4 h-4 mx-auto text-green\" viewBox=\"0 0 24 24\"><g fill=\"none\" stroke=\"currentColor\" stroke-linecap=\"round\" stroke-width=\"2\"><path stroke-miterlimit=\"10\" d=\"M6.343 17.657L17.657 6.343\"></path> <path stroke-linejoin=\"round\" d=\"M5.899 7.267v9.296A1.53 1.53 0 0 0 7.437 18.1h9.296\"></path></g></svg></div>")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		return templ_7745c5c3_Err
	})
}

func SentIcon() templ.Component {
	return templruntime.GeneratedTemplate(func(templ_7745c5c3_Input templruntime.GeneratedComponentInput) (templ_7745c5c3_Err error) {
		templ_7745c5c3_W, ctx := templ_7745c5c3_Input.Writer, templ_7745c5c3_Input.Context
		if templ_7745c5c3_CtxErr := ctx.Err(); templ_7745c5c3_CtxErr != nil {
			return templ_7745c5c3_CtxErr
		}
		templ_7745c5c3_Buffer, templ_7745c5c3_IsBuffer := templruntime.GetBuffer(templ_7745c5c3_W)
		if !templ_7745c5c3_IsBuffer {
			defer func() {
				templ_7745c5c3_BufErr := templruntime.ReleaseBuffer(templ_7745c5c3_Buffer)
				if templ_7745c5c3_Err == nil {
					templ_7745c5c3_Err = templ_7745c5c3_BufErr
				}
			}()
		}
		ctx = templ.InitializeContext(ctx)
		templ_7745c5c3_Var3 := templ.GetChildren(ctx)
		if templ_7745c5c3_Var3 == nil {
			templ_7745c5c3_Var3 = templ.NopComponent
		}
		ctx = templ.ClearChildren(ctx)
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("<div class=\"flex items-center w-8 h-8 bg-redbg/10 rounded-full mr-2\"><svg xmlns=\"http://www.w3.org/2000/svg\" class=\"w-4 h-4 mx-auto text-red\" viewBox=\"0 0 24 24\"><g fill=\"none\" stroke=\"currentColor\" stroke-linecap=\"round\" stroke-width=\"2\"><path stroke-miterlimit=\"10\" d=\"M17.657 6.343L6.343 17.657\"></path> <path stroke-linejoin=\"round\" d=\"M18.101 16.733V7.437A1.53 1.53 0 0 0 16.563 5.9H7.267\"></path></g></svg></div>")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		return templ_7745c5c3_Err
	})
}

func SwapIcon() templ.Component {
	return templruntime.GeneratedTemplate(func(templ_7745c5c3_Input templruntime.GeneratedComponentInput) (templ_7745c5c3_Err error) {
		templ_7745c5c3_W, ctx := templ_7745c5c3_Input.Writer, templ_7745c5c3_Input.Context
		if templ_7745c5c3_CtxErr := ctx.Err(); templ_7745c5c3_CtxErr != nil {
			return templ_7745c5c3_CtxErr
		}
		templ_7745c5c3_Buffer, templ_7745c5c3_IsBuffer := templruntime.GetBuffer(templ_7745c5c3_W)
		if !templ_7745c5c3_IsBuffer {
			defer func() {
				templ_7745c5c3_BufErr := templruntime.ReleaseBuffer(templ_7745c5c3_Buffer)
				if templ_7745c5c3_Err == nil {
					templ_7745c5c3_Err = templ_7745c5c3_BufErr
				}
			}()
		}
		ctx = templ.InitializeContext(ctx)
		templ_7745c5c3_Var4 := templ.GetChildren(ctx)
		if templ_7745c5c3_Var4 == nil {
			templ_7745c5c3_Var4 = templ.NopComponent
		}
		ctx = templ.ClearChildren(ctx)
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("<div class=\"flex items-center w-8 h-8 bg-white/10 rounded-full mr-2\"><svg xmlns=\"http://www.w3.org/2000/svg\" class=\"w-4 h-4 mx-auto text-white\" viewBox=\"0 0 21 21\"><g fill=\"none\" fill-rule=\"evenodd\" stroke=\"currentColor\" stroke-linecap=\"round\" stroke-linejoin=\"round\" stroke-width=\"2\"><path d=\"M13 8h5V3\"></path> <path d=\"M18 8c-2.837-3.333-5.67-5-8.5-5S4.17 4 2 6m4.5 5.5h-5v5\"></path> <path d=\"M1.5 11.5c2.837 3.333 5.67 5 8.5 5s5.33-1 7.5-3\"></path></g></svg></div>")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		return templ_7745c5c3_Err
	})
}

func WaitingIcon() templ.Component {
	return templruntime.GeneratedTemplate(func(templ_7745c5c3_Input templruntime.GeneratedComponentInput) (templ_7745c5c3_Err error) {
		templ_7745c5c3_W, ctx := templ_7745c5c3_Input.Writer, templ_7745c5c3_Input.Context
		if templ_7745c5c3_CtxErr := ctx.Err(); templ_7745c5c3_CtxErr != nil {
			return templ_7745c5c3_CtxErr
		}
		templ_7745c5c3_Buffer, templ_7745c5c3_IsBuffer := templruntime.GetBuffer(templ_7745c5c3_W)
		if !templ_7745c5c3_IsBuffer {
			defer func() {
				templ_7745c5c3_BufErr := templruntime.ReleaseBuffer(templ_7745c5c3_Buffer)
				if templ_7745c5c3_Err == nil {
					templ_7745c5c3_Err = templ_7745c5c3_BufErr
				}
			}()
		}
		ctx = templ.InitializeContext(ctx)
		templ_7745c5c3_Var5 := templ.GetChildren(ctx)
		if templ_7745c5c3_Var5 == nil {
			templ_7745c5c3_Var5 = templ.NopComponent
		}
		ctx = templ.ClearChildren(ctx)
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("<div class=\"flex items-center w-8 h-8 bg-yellowbg/10 rounded-full mr-2\"><svg xmlns=\"http://www.w3.org/2000/svg\" class=\"w-4 h-4 mx-auto text-yellow\" viewBox=\"0 0 24 24\"><path fill=\"none\" stroke=\"currentColor\" stroke-linecap=\"round\" stroke-linejoin=\"round\" stroke-width=\"2\" d=\"M12 3v3m6.366-.366l-2.12 2.12M21 12h-3m.366 6.366l-2.12-2.12M12 21v-3m-6.366.366l2.12-2.12M3 12h3m-.366-6.366l2.12 2.12\"></path></svg></div>")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		return templ_7745c5c3_Err
	})
}

func HistoryLine(tx types.Transaction) templ.Component {
	return templruntime.GeneratedTemplate(func(templ_7745c5c3_Input templruntime.GeneratedComponentInput) (templ_7745c5c3_Err error) {
		templ_7745c5c3_W, ctx := templ_7745c5c3_Input.Writer, templ_7745c5c3_Input.Context
		if templ_7745c5c3_CtxErr := ctx.Err(); templ_7745c5c3_CtxErr != nil {
			return templ_7745c5c3_CtxErr
		}
		templ_7745c5c3_Buffer, templ_7745c5c3_IsBuffer := templruntime.GetBuffer(templ_7745c5c3_W)
		if !templ_7745c5c3_IsBuffer {
			defer func() {
				templ_7745c5c3_BufErr := templruntime.ReleaseBuffer(templ_7745c5c3_Buffer)
				if templ_7745c5c3_Err == nil {
					templ_7745c5c3_Err = templ_7745c5c3_BufErr
				}
			}()
		}
		ctx = templ.InitializeContext(ctx)
		templ_7745c5c3_Var6 := templ.GetChildren(ctx)
		if templ_7745c5c3_Var6 == nil {
			templ_7745c5c3_Var6 = templ.NopComponent
		}
		ctx = templ.ClearChildren(ctx)
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("<div txid=\"")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		var templ_7745c5c3_Var7 string
		templ_7745c5c3_Var7, templ_7745c5c3_Err = templ.JoinStringErrs(tx.Txid)
		if templ_7745c5c3_Err != nil {
			return templ.Error{Err: templ_7745c5c3_Err, FileName: `internal/interface/web/templates/pages/history.templ`, Line: 78, Col: 19}
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var7))
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("\" onclick=\"showTx(this)\"><div class=\"flex justify-between cursor-pointer p-3 rounded-lg hover:bg-white/10\"><div class=\"flex w-64\">")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = HistoryIcon(tx.Kind, tx.Status).Render(ctx, templ_7745c5c3_Buffer)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		if tx.UnixDate != 0 {
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("<p class=\"leading-4\">")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			var templ_7745c5c3_Var8 string
			templ_7745c5c3_Var8, templ_7745c5c3_Err = templ.JoinStringErrs(tx.Day)
			if templ_7745c5c3_Err != nil {
				return templ.Error{Err: templ_7745c5c3_Err, FileName: `internal/interface/web/templates/pages/history.templ`, Line: 83, Col: 36}
			}
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var8))
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("<br><span class=\"text-white/50\">")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			var templ_7745c5c3_Var9 string
			templ_7745c5c3_Var9, templ_7745c5c3_Err = templ.JoinStringErrs(tx.Hour)
			if templ_7745c5c3_Err != nil {
				return templ.Error{Err: templ_7745c5c3_Err, FileName: `internal/interface/web/templates/pages/history.templ`, Line: 83, Col: 79}
			}
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var9))
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("</span></p>")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("</div>")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		if tx.Status == "pending" {
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("<div class=\"flex items-center\"><p class=\"bg-orange/20 px-2 py-1 rounded-md text-sm text-orange\">Pending</p></div>")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
		}
		if tx.Status == "failed" {
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("<div class=\"flex items-center\"><p class=\"bg-red/20 px-2 py-1 rounded-md text-sm text-red\">Failed</p></div>")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
		}
		if tx.Status == "unconfirmed" {
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("<div class=\"flex items-center\"><p class=\"bg-white/20 px-2 py-1 rounded-md text-sm text-white\">Unconfirmed</p></div>")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("<div class=\"flex w-64\"><p class=\"leading-4 text-end w-full\">")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		var templ_7745c5c3_Var10 = []any{
			templ.KV("text-red", tx.Status != "pending" && tx.Kind == "sent"),
			templ.KV("text-green", tx.Status != "pending" && tx.Kind == "received"),
			templ.KV("text-yellow", tx.Status == "pending"),
			templ.KV("text-white", tx.Status == "unconfirmed"),
		}
		templ_7745c5c3_Err = templ.RenderCSSItems(ctx, templ_7745c5c3_Buffer, templ_7745c5c3_Var10...)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("<span class=\"")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		var templ_7745c5c3_Var11 string
		templ_7745c5c3_Var11, templ_7745c5c3_Err = templ.JoinStringErrs(templ.CSSClasses(templ_7745c5c3_Var10).String())
		if templ_7745c5c3_Err != nil {
			return templ.Error{Err: templ_7745c5c3_Err, FileName: `internal/interface/web/templates/pages/history.templ`, Line: 1, Col: 0}
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var11))
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("\"><span class=\"cryptoAmount\" sats=\"")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		var templ_7745c5c3_Var12 string
		templ_7745c5c3_Var12, templ_7745c5c3_Err = templ.JoinStringErrs(tx.Amount)
		if templ_7745c5c3_Err != nil {
			return templ.Error{Err: templ_7745c5c3_Err, FileName: `internal/interface/web/templates/pages/history.templ`, Line: 109, Col: 50}
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var12))
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("\">")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		var templ_7745c5c3_Var13 string
		templ_7745c5c3_Var13, templ_7745c5c3_Err = templ.JoinStringErrs(tx.Amount)
		if templ_7745c5c3_Err != nil {
			return templ.Error{Err: templ_7745c5c3_Err, FileName: `internal/interface/web/templates/pages/history.templ`, Line: 109, Col: 62}
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var13))
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(" SATS</span></span><br><span class=\"text-white/50 fiatAmount\" sats=\"")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		var templ_7745c5c3_Var14 string
		templ_7745c5c3_Var14, templ_7745c5c3_Err = templ.JoinStringErrs(tx.Amount)
		if templ_7745c5c3_Err != nil {
			return templ.Error{Err: templ_7745c5c3_Err, FileName: `internal/interface/web/templates/pages/history.templ`, Line: 112, Col: 61}
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var14))
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("\">&nbsp;</span></p></div></div></div>")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		return templ_7745c5c3_Err
	})
}

func HistoryEmpty() templ.Component {
	return templruntime.GeneratedTemplate(func(templ_7745c5c3_Input templruntime.GeneratedComponentInput) (templ_7745c5c3_Err error) {
		templ_7745c5c3_W, ctx := templ_7745c5c3_Input.Writer, templ_7745c5c3_Input.Context
		if templ_7745c5c3_CtxErr := ctx.Err(); templ_7745c5c3_CtxErr != nil {
			return templ_7745c5c3_CtxErr
		}
		templ_7745c5c3_Buffer, templ_7745c5c3_IsBuffer := templruntime.GetBuffer(templ_7745c5c3_W)
		if !templ_7745c5c3_IsBuffer {
			defer func() {
				templ_7745c5c3_BufErr := templruntime.ReleaseBuffer(templ_7745c5c3_Buffer)
				if templ_7745c5c3_Err == nil {
					templ_7745c5c3_Err = templ_7745c5c3_BufErr
				}
			}()
		}
		ctx = templ.InitializeContext(ctx)
		templ_7745c5c3_Var15 := templ.GetChildren(ctx)
		if templ_7745c5c3_Var15 == nil {
			templ_7745c5c3_Var15 = templ.NopComponent
		}
		ctx = templ.ClearChildren(ctx)
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("<div class=\"flex flex-col items-center w-full\"><svg width=\"170\" height=\"102\" viewBox=\"0 0 170 102\" fill=\"none\" xmlns=\"http://www.w3.org/2000/svg\"><path d=\"M1.53789 33.0852C0.688536 29.9154 2.56965 26.6572 5.73948 25.8078L53.0902 13.1202C56.26 12.2709 59.5182 14.152 60.3675 17.3218L77.0921 79.7386C77.9414 82.9085 76.0603 86.1667 72.8905 87.016L25.5398 99.7036C22.37 100.553 19.1118 98.6718 18.2624 95.502L1.53789 33.0852Z\" fill=\"#121212\"></path> <path d=\"M20.0836 98.4319C19.5064 97.9266 19.0225 97.3059 18.6718 96.5929L19.005 96.4289C18.8469 96.1075 18.7176 95.7657 18.6211 95.4059L18.241 93.9873L17.8823 94.0834L17.1221 91.2463L17.4808 91.1502L16.7206 88.313L16.3619 88.4092L15.6017 85.572L15.9604 85.4759L15.2002 82.6388L14.8415 82.7349L14.0813 79.8978L14.44 79.8017L13.6798 76.9645L13.3211 77.0607L12.5609 74.2235L12.9196 74.1274L12.1594 71.2903L11.8007 71.3864L11.0405 68.5493L11.3992 68.4532L10.639 65.616L10.2803 65.7121L9.52005 62.875L9.87877 62.7789L9.11856 59.9418L8.75985 60.0379L7.99964 57.2008L8.35836 57.1046L7.59815 54.2675L7.23943 54.3636L6.47923 51.5265L6.83795 51.4304L6.07774 48.5933L5.71902 48.6894L4.95882 45.8523L5.31753 45.7561L4.55733 42.919L4.19861 43.0151L3.4384 40.178L3.79712 40.0819L3.03692 37.2448L2.6782 37.3409L1.91799 34.5037L2.27671 34.4076L1.89661 32.9891C1.80019 32.6292 1.74135 32.2686 1.71755 31.9111L1.347 31.9358C1.2942 31.1429 1.40288 30.3635 1.65014 29.6372L2.0017 29.7569C2.23737 29.0647 2.60735 28.4238 3.089 27.8736L2.80957 27.629C3.3149 27.0517 3.93555 26.5679 4.64862 26.2172L4.81253 26.5504C5.13401 26.3923 5.47578 26.2629 5.83559 26.1665L7.3153 25.77L7.21918 25.4113L10.1786 24.6184L10.2747 24.9771L13.2341 24.1841L13.138 23.8254L16.0974 23.0324L16.1936 23.3911L19.153 22.5982L19.0569 22.2394L22.0163 21.4465L22.1124 21.8052L25.0718 21.0122L24.9757 20.6535L27.9351 19.8605L28.0312 20.2192L30.9906 19.4263L30.8945 19.0675L33.8539 18.2746L33.9501 18.6333L36.9095 17.8403L36.8134 17.4816L39.7728 16.6886L39.8689 17.0473L42.8283 16.2544L42.7322 15.8956L45.6916 15.1027L45.7877 15.4614L48.7472 14.6684L48.651 14.3097L51.6105 13.5167L51.7066 13.8754L53.1863 13.479C53.5461 13.3825 53.9068 13.3237 54.2642 13.2999L54.2396 12.9293C55.0325 12.8765 55.8119 12.9852 56.5381 13.2325L56.4185 13.584C57.1107 13.8197 57.7515 14.1897 58.3017 14.6713L58.5463 14.3919C59.1236 14.8972 59.6074 15.5179 59.9581 16.231L59.6249 16.3949C59.783 16.7164 59.9124 17.0581 60.0088 17.4179L60.3889 18.8365L60.7476 18.7404L61.5078 21.5775L61.1491 21.6736L61.9093 24.5108L62.268 24.4147L63.0283 27.2518L62.6695 27.3479L63.4297 30.185L63.7885 30.0889L64.5487 32.926L64.1899 33.0222L64.9502 35.8593L65.3089 35.7632L66.0691 38.6003L65.7104 38.6964L66.4706 41.5335L66.8293 41.4374L67.5895 44.2745L67.2308 44.3707L67.991 47.2078L68.3497 47.1117L69.1099 49.9488L68.7512 50.0449L69.5114 52.882L69.8701 52.7859L70.6303 55.6231L70.2716 55.7192L71.0318 58.5563L71.3905 58.4602L72.1507 61.2973L71.792 61.3934L72.5522 64.2306L72.9109 64.1344L73.6711 66.9716L73.3124 67.0677L74.0726 69.9048L74.4313 69.8087L75.1915 72.6458L74.8328 72.7419L75.593 75.5791L75.9518 75.4829L76.712 78.3201L76.3532 78.4162L76.7333 79.8348C76.8298 80.1946 76.8886 80.5552 76.9124 80.9127L77.283 80.888C77.3358 81.6809 77.2271 82.4604 76.9798 83.1866L76.6283 83.0669C76.3926 83.7591 76.0226 84.4 75.541 84.9502L75.8204 85.1948C75.3151 85.7721 74.6944 86.2559 73.9813 86.6066L73.8174 86.2734C73.4959 86.4315 73.1542 86.5609 72.7944 86.6573L71.3147 87.0538L71.4108 87.4125L68.4514 88.2055L68.3552 87.8468L65.3958 88.6397L65.4919 88.9984L62.5325 89.7914L62.4364 89.4327L59.477 90.2257L59.5731 90.5844L56.6137 91.3774L56.5176 91.0186L53.5581 91.8116L53.6543 92.1703L50.6948 92.9633L50.5987 92.6046L47.6393 93.3976L47.7354 93.7563L44.776 94.5493L44.6799 94.1905L41.7205 94.9835L41.8166 95.3422L38.8572 96.1352L38.7611 95.7765L35.8016 96.5695L35.8978 96.9282L32.9383 97.7212L32.8422 97.3624L29.8828 98.1554L29.9789 98.5141L27.0195 99.3071L26.9234 98.9484L25.4437 99.3449C25.0839 99.4413 24.7232 99.5001 24.3657 99.5239L24.3904 99.8945C23.5975 99.9473 22.8181 99.8386 22.0918 99.5913L22.2115 99.2398C21.5193 99.0041 20.8784 98.6341 20.3282 98.1525L20.0836 98.4319Z\" stroke=\"#FBFBFB\" stroke-opacity=\"0.1\" stroke-width=\"0.742743\" stroke-dasharray=\"2.97 2.97\"></path> <g clip-path=\"url(#clip0_352_3581)\"><path d=\"M37.7074 47.5249L38.4839 50.4227C38.5354 50.6148 38.5084 50.8195 38.409 50.9918C38.3095 51.1641 38.1457 51.2898 37.9536 51.3412C37.7614 51.3927 37.5567 51.3658 37.3844 51.2663C37.2122 51.1669 37.0865 51.0031 37.035 50.8109L36.2585 47.9131C36.2071 47.721 36.234 47.5163 36.3335 47.344C36.4329 47.1718 36.5967 47.0461 36.7889 46.9946C36.981 46.9431 37.1857 46.97 37.358 47.0695C37.5302 47.169 37.6559 47.3328 37.7074 47.5249ZM42.5061 51.9412C42.6014 51.9156 42.6906 51.8716 42.7688 51.8115C42.8469 51.7514 42.9125 51.6765 42.9617 51.5911L44.4621 48.9936C44.5616 48.8212 44.5886 48.6164 44.5371 48.4242C44.4855 48.2319 44.3598 48.068 44.1874 47.9685C44.0151 47.869 43.8102 47.842 43.618 47.8935C43.4257 47.9451 43.2618 48.0708 43.1623 48.2432L41.6631 50.8413C41.5888 50.9697 41.5543 51.1173 41.564 51.2653C41.5736 51.4134 41.6269 51.5552 41.7171 51.6729C41.8074 51.7907 41.9305 51.879 42.0709 51.9268C42.2114 51.9745 42.3628 51.9795 42.5061 51.9412ZM47.8116 53.3585L44.9138 54.135C44.7217 54.1865 44.5578 54.3122 44.4584 54.4844C44.3589 54.6567 44.332 54.8614 44.3835 55.0536C44.4349 55.2457 44.5606 55.4095 44.7329 55.509C44.9052 55.6084 45.1099 55.6354 45.302 55.5839L48.1998 54.8074C48.3919 54.7559 48.5558 54.6302 48.6552 54.458C48.7547 54.2857 48.7816 54.081 48.7301 53.8889C48.6787 53.6967 48.553 53.5329 48.3807 53.4335C48.2084 53.334 48.0037 53.3071 47.8116 53.3585ZM44.8834 58.7631C44.7116 58.6696 44.51 58.647 44.3218 58.7001C44.1335 58.7531 43.9734 58.8776 43.8756 59.047C43.7778 59.2164 43.7501 59.4172 43.7983 59.6068C43.8465 59.7963 43.9668 59.9596 44.1337 60.0617L46.7311 61.5621C46.9035 61.6616 47.1083 61.6886 47.3005 61.6371C47.4928 61.5855 47.6567 61.4598 47.7562 61.2874C47.8557 61.1151 47.8827 60.9102 47.8312 60.718C47.7797 60.5257 47.6539 60.3618 47.4815 60.2623L44.8834 58.7631ZM40.6712 61.4835C40.479 61.535 40.3152 61.6606 40.2158 61.8329C40.1163 62.0052 40.0893 62.2099 40.1408 62.402L40.9173 65.2998C40.9688 65.4919 41.0945 65.6558 41.2667 65.7552C41.439 65.8547 41.6437 65.8816 41.8358 65.8301C42.028 65.7787 42.1918 65.653 42.2912 65.4807C42.3907 65.3084 42.4177 65.1037 42.3662 64.9116L41.5897 62.0138C41.5382 61.8217 41.4125 61.6578 41.2403 61.5584C41.068 61.4589 40.8633 61.432 40.6712 61.4835ZM35.663 61.2337L34.1626 63.8311C34.0631 64.0035 34.0361 64.2083 34.0877 64.4006C34.1392 64.5928 34.2649 64.7567 34.4373 64.8562C34.6097 64.9557 34.8145 64.9827 35.0067 64.9312C35.199 64.8797 35.3629 64.7539 35.4624 64.5815L36.9617 61.9834C37.0551 61.8116 37.0777 61.61 37.0246 61.4218C36.9716 61.2335 36.8471 61.0734 36.6777 60.9756C36.5084 60.8778 36.3075 60.8501 36.1179 60.8983C35.9284 60.9465 35.7651 61.0668 35.663 61.2337ZM34.2412 57.7712C34.1898 57.579 34.0641 57.4152 33.8918 57.3158C33.7195 57.2163 33.5148 57.1893 33.3227 57.2408L30.4249 58.0173C30.2328 58.0688 30.069 58.1945 29.9695 58.3667C29.87 58.539 29.8431 58.7437 29.8946 58.9358C29.9461 59.128 30.0718 59.2918 30.244 59.3912C30.4163 59.4907 30.621 59.5177 30.8131 59.4662L33.7109 58.6897C33.9031 58.6382 34.0669 58.5125 34.1663 58.3403C34.2658 58.168 34.2927 57.9633 34.2412 57.7712ZM31.8936 51.2626C31.7212 51.1631 31.5164 51.1361 31.3242 51.1877C31.1319 51.2392 30.968 51.3649 30.8685 51.5373C30.769 51.7097 30.742 51.9145 30.7935 52.1067C30.845 52.299 30.9708 52.4629 31.1432 52.5624L33.7413 54.0617C33.9131 54.1551 34.1147 54.1777 34.3029 54.1246C34.4912 54.0716 34.6513 53.9471 34.7491 53.7777C34.8469 53.6084 34.8746 53.4075 34.8264 53.2179C34.7782 53.0284 34.6579 52.8651 34.4911 52.763L31.8936 51.2626Z\" fill=\"#F2F2F2\" fill-opacity=\"0.4\"></path></g> <path d=\"M109.39 17.2187C110.242 14.0361 113.514 12.1474 116.696 13.0001L164.238 25.7389C167.42 26.5916 169.309 29.863 168.456 33.0456L151.857 94.9937C151.005 98.1763 147.733 100.065 144.551 99.2122L97.0091 86.4735C93.8265 85.6207 91.9378 82.3494 92.7906 79.1668L109.39 17.2187Z\" fill=\"#121212\"></path> <path d=\"M92.9033 82.6286C92.655 81.8995 92.5459 81.1169 92.5989 80.3208L92.9709 80.3456C92.9948 79.9867 93.0539 79.6245 93.1507 79.2633L93.528 77.8554L93.1678 77.7588L93.9223 74.943L94.2825 75.0395L95.037 72.2237L94.6768 72.1272L95.4313 69.3114L95.7915 69.4079L96.546 66.5921L96.1858 66.4956L96.9403 63.6797L97.3005 63.7762L98.055 60.9604L97.6948 60.8639L98.4493 58.0481L98.8095 58.1446L99.564 55.3288L99.2038 55.2323L99.9583 52.4164L100.318 52.513L101.073 49.6971L100.713 49.6006L101.467 46.7848L101.827 46.8813L102.582 44.0655L102.222 43.969L102.976 41.1532L103.336 41.2497L104.091 38.4338L103.731 38.3373L104.485 35.5215L104.845 35.618L105.6 32.8022L105.24 32.7057L105.994 29.8899L106.354 29.9864L107.109 27.1706L106.749 27.074L107.503 24.2582L107.863 24.3547L108.618 21.5389L108.258 21.4424L109.012 18.6266L109.372 18.7231L109.75 17.3152C109.846 16.9539 109.976 16.6108 110.135 16.288L109.801 16.1234C110.153 15.4075 110.638 14.7843 111.218 14.2769L111.464 14.5575C112.016 14.0739 112.659 13.7024 113.355 13.4658L113.234 13.1128C113.964 12.8646 114.746 12.7555 115.542 12.8085L115.517 13.1805C115.876 13.2044 116.238 13.2635 116.6 13.3603L118.085 13.7584L118.182 13.3982L121.153 14.1944L121.057 14.5546L124.028 15.3507L124.125 14.9906L127.096 15.7867L126.999 16.1469L129.971 16.9431L130.067 16.5829L133.039 17.3791L132.942 17.7392L135.913 18.5354L136.01 18.1752L138.981 18.9714L138.885 19.3316L141.856 20.1278L141.953 19.7676L144.924 20.5638L144.828 20.9239L147.799 21.7201L147.895 21.3599L150.867 22.1561L150.77 22.5163L153.742 23.3124L153.838 22.9523L156.809 23.7484L156.713 24.1086L159.684 24.9048L159.781 24.5446L162.752 25.3408L162.656 25.7009L164.141 26.099C164.503 26.1958 164.846 26.3257 165.168 26.4845L165.333 26.1499C166.049 26.502 166.672 26.9878 167.179 27.5674L166.899 27.813C167.383 28.3654 167.754 29.0089 167.991 29.7039L168.344 29.5837C168.592 30.3129 168.701 31.0954 168.648 31.8915L168.276 31.8668C168.252 32.2257 168.193 32.5878 168.096 32.9491L167.719 34.357L168.079 34.4535L167.325 37.2693L166.964 37.1728L166.21 39.9886L166.57 40.0851L165.816 42.901L165.455 42.8045L164.701 45.6203L165.061 45.7168L164.307 48.5326L163.946 48.4361L163.192 51.2519L163.552 51.3484L162.798 54.1642L162.437 54.0677L161.683 56.8836L162.043 56.9801L161.289 59.7959L160.928 59.6994L160.174 62.5152L160.534 62.6117L159.78 65.4275L159.419 65.331L158.665 68.1468L159.025 68.2434L158.271 71.0592L157.91 70.9627L157.156 73.7785L157.516 73.875L156.762 76.6908L156.401 76.5943L155.647 79.4101L156.007 79.5066L155.253 82.3225L154.892 82.226L154.138 85.0418L154.498 85.1383L153.744 87.9541L153.383 87.8576L152.629 90.6734L152.989 90.7699L152.235 93.5858L151.874 93.4892L151.497 94.8972C151.4 95.2584 151.27 95.6016 151.112 95.9244L151.446 96.0889C151.094 96.8049 150.608 97.428 150.029 97.9354L149.783 97.6548C149.231 98.1384 148.587 98.5099 147.892 98.7465L148.013 99.0995C147.283 99.3478 146.501 99.4569 145.705 99.4039L145.729 99.0318C145.371 99.0079 145.008 98.9488 144.647 98.852L143.161 98.4539L143.065 98.8141L140.094 98.0179L140.19 97.6578L137.219 96.8616L137.122 97.2218L134.151 96.4256L134.247 96.0654L131.276 95.2693L131.18 95.6294L128.208 94.8333L128.305 94.4731L125.333 93.6769L125.237 94.0371L122.266 93.2409L122.362 92.8808L119.391 92.0846L119.294 92.4447L116.323 91.6486L116.419 91.2884L113.448 90.4922L113.352 90.8524L110.38 90.0562L110.477 89.6961L107.505 88.8999L107.409 89.2601L104.437 88.4639L104.534 88.1037L101.563 87.3076L101.466 87.6677L98.4948 86.8716L98.5913 86.5114L97.1056 86.1133C96.7443 86.0165 96.4012 85.8866 96.0784 85.7279L95.9138 86.0624C95.1979 85.7103 94.5747 85.2245 94.0674 84.6449L94.3479 84.3993C93.8643 83.8469 93.4929 83.2035 93.2562 82.5085L92.9033 82.6286Z\" stroke=\"#FBFBFB\" stroke-opacity=\"0.1\" stroke-width=\"0.745737\" stroke-dasharray=\"2.98 2.98\"></path> <path d=\"M138.696 52.0585L136.172 61.4762C136.121 61.6684 135.995 61.8322 135.823 61.9316C135.651 62.0311 135.446 62.058 135.254 62.0066C135.062 61.9551 134.898 61.8294 134.798 61.6571C134.699 61.4849 134.672 61.2801 134.724 61.088L136.778 53.4189L123.65 60.9995C123.477 61.099 123.273 61.126 123.08 61.0745C122.888 61.023 122.724 60.8972 122.625 60.7248C122.525 60.5525 122.498 60.3476 122.55 60.1554C122.601 59.9632 122.727 59.7993 122.899 59.6997L136.029 52.1202L128.36 50.0653C128.167 50.0138 128.004 49.8881 127.904 49.7159C127.805 49.5436 127.778 49.3389 127.829 49.1467C127.881 48.9546 128.006 48.7908 128.179 48.6913C128.351 48.5919 128.556 48.5649 128.748 48.6164L138.166 51.1399C138.358 51.1914 138.522 51.3171 138.621 51.4893C138.72 51.6616 138.747 51.8663 138.696 52.0585Z\" fill=\"#F2F2F2\" fill-opacity=\"0.4\"></path> <rect x=\"51.5371\" width=\"72.2617\" height=\"90.7678\" rx=\"7.04992\" fill=\"#121212\"></rect> <rect x=\"51.9777\" y=\"0.44062\" width=\"71.3805\" height=\"89.8865\" rx=\"6.6093\" stroke=\"#FBFBFB\" stroke-opacity=\"0.1\" stroke-width=\"0.88124\"></rect> <path d=\"M95.2851 39.0033L82.778 51.5093H92.041C92.2731 51.5093 92.4956 51.6015 92.6597 51.7656C92.8238 51.9297 92.916 52.1522 92.916 52.3843C92.916 52.6163 92.8238 52.8389 92.6597 53.003C92.4956 53.1671 92.2731 53.2593 92.041 53.2593H80.666C80.434 53.2593 80.2114 53.1671 80.0473 53.003C79.8832 52.8389 79.791 52.6163 79.791 52.3843V41.0093C79.791 40.7772 79.8832 40.5547 80.0473 40.3906C80.2114 40.2265 80.434 40.1343 80.666 40.1343C80.8981 40.1343 81.1206 40.2265 81.2847 40.3906C81.4488 40.5547 81.541 40.7772 81.541 41.0093V50.2722L94.047 37.7652C94.1283 37.6839 94.2248 37.6194 94.331 37.5754C94.4372 37.5314 94.551 37.5088 94.666 37.5088C94.781 37.5088 94.8948 37.5314 95.0011 37.5754C95.1073 37.6194 95.2038 37.6839 95.2851 37.7652C95.3664 37.8465 95.4309 37.943 95.4749 38.0492C95.5189 38.1555 95.5415 38.2693 95.5415 38.3843C95.5415 38.4992 95.5189 38.6131 95.4749 38.7193C95.4309 38.8255 95.3664 38.922 95.2851 39.0033Z\" fill=\"#F2F2F2\" fill-opacity=\"0.4\"></path> <defs><clipPath id=\"clip0_352_3581\"><rect width=\"24\" height=\"24\" fill=\"white\" transform=\"translate(24.6172 47.9268) rotate(-15)\"></rect></clipPath></defs></svg><p class=\"font-bold\">Looks like you don't have any transactions</p><p class=\"text-white/50\">Send, receive, or swap your first BTC on Ark.</p></div>")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		return templ_7745c5c3_Err
	})
}

func HistoryBodyContent(currentBalance, arkAddress string, transactions []types.Transaction, online bool) templ.Component {
	return templruntime.GeneratedTemplate(func(templ_7745c5c3_Input templruntime.GeneratedComponentInput) (templ_7745c5c3_Err error) {
		templ_7745c5c3_W, ctx := templ_7745c5c3_Input.Writer, templ_7745c5c3_Input.Context
		if templ_7745c5c3_CtxErr := ctx.Err(); templ_7745c5c3_CtxErr != nil {
			return templ_7745c5c3_CtxErr
		}
		templ_7745c5c3_Buffer, templ_7745c5c3_IsBuffer := templruntime.GetBuffer(templ_7745c5c3_W)
		if !templ_7745c5c3_IsBuffer {
			defer func() {
				templ_7745c5c3_BufErr := templruntime.ReleaseBuffer(templ_7745c5c3_Buffer)
				if templ_7745c5c3_Err == nil {
					templ_7745c5c3_Err = templ_7745c5c3_BufErr
				}
			}()
		}
		ctx = templ.InitializeContext(ctx)
		templ_7745c5c3_Var16 := templ.GetChildren(ctx)
		if templ_7745c5c3_Var16 == nil {
			templ_7745c5c3_Var16 = templ.NopComponent
		}
		ctx = templ.ClearChildren(ctx)
		templ_7745c5c3_Err = components.Hero(arkAddress, currentBalance, online).Render(ctx, templ_7745c5c3_Buffer)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		if len(transactions) == 0 {
			templ_7745c5c3_Err = HistoryEmpty().Render(ctx, templ_7745c5c3_Buffer)
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
		} else {
			for _, tx := range transactions {
				templ_7745c5c3_Err = HistoryLine(tx).Render(ctx, templ_7745c5c3_Buffer)
				if templ_7745c5c3_Err != nil {
					return templ_7745c5c3_Err
				}
			}
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(" <script>\n\t    const showTx = (el) => {\n\t  \t\tconst txid = el.getAttribute(\"txid\")\n\t  \t\tif (txid) redirect(`/tx/${txid}`)\n\t  \t}\n\t  </script>")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
		}
		return templ_7745c5c3_Err
	})
}

var _ = templruntime.GeneratedTemplate
