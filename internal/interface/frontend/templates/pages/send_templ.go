// Code generated by templ - DO NOT EDIT.

// templ: version: v0.2.747
package pages

//lint:file-ignore SA4006 This context is only used if a nested component is present.

import "github.com/a-h/templ"
import templruntime "github.com/a-h/templ/runtime"

import (
	"github.com/ArkLabsHQ/ark-wallet/templates/components"
)

func ArrowDownIcon() templ.Component {
	return templruntime.GeneratedTemplate(func(templ_7745c5c3_Input templruntime.GeneratedComponentInput) (templ_7745c5c3_Err error) {
		templ_7745c5c3_W, ctx := templ_7745c5c3_Input.Writer, templ_7745c5c3_Input.Context
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
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("<svg xmlns=\"http://www.w3.org/2000/svg\" class=\"h-4 w-4 text-offwhite/50\" viewBox=\"0 0 20 20\"><path fill=\"currentColor\" d=\"M10.103 12.778L16.81 6.08a.69.69 0 0 1 .99.012a.726.726 0 0 1-.012 1.012l-7.203 7.193a.69.69 0 0 1-.985-.006L2.205 6.72a.727.727 0 0 1 0-1.01a.69.69 0 0 1 .99 0l6.908 7.068Z\"></path></svg>")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		return templ_7745c5c3_Err
	})
}

func PasteIcon() templ.Component {
	return templruntime.GeneratedTemplate(func(templ_7745c5c3_Input templruntime.GeneratedComponentInput) (templ_7745c5c3_Err error) {
		templ_7745c5c3_W, ctx := templ_7745c5c3_Input.Writer, templ_7745c5c3_Input.Context
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
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("<svg xmlns=\"http://www.w3.org/2000/svg\" class=\"h-7 w-7 text-offwhite/50\" viewBox=\"0 0 24 24\"><path fill=\"currentColor\" d=\"M5.615 20q-.67 0-1.143-.472Q4 19.056 4 18.385V5.615q0-.67.472-1.143Q4.944 4 5.615 4h4.637q.14-.587.623-.986T12 2.615q.654 0 1.134.4q.48.398.62.985h4.63q.672 0 1.144.472q.472.472.472 1.143v12.77q0 .67-.472 1.143q-.472.472-1.143.472H5.615Zm0-1h12.77q.23 0 .423-.192q.192-.193.192-.423V5.615q0-.23-.192-.423Q18.615 5 18.385 5H16v2.23H8V5H5.615q-.23 0-.423.192Q5 5.385 5 5.615v12.77q0 .23.192.423q.193.192.423.192ZM12 5.23q.348 0 .578-.229q.23-.23.23-.578t-.23-.578q-.23-.23-.578-.23t-.578.23q-.23.23-.23.578t.23.578q.23.23.578.23Z\"></path></svg>")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		return templ_7745c5c3_Err
	})
}

func SendBodyContent(currentBalance string) templ.Component {
	return templruntime.GeneratedTemplate(func(templ_7745c5c3_Input templruntime.GeneratedComponentInput) (templ_7745c5c3_Err error) {
		templ_7745c5c3_W, ctx := templ_7745c5c3_Input.Writer, templ_7745c5c3_Input.Context
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
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("<div class=\"p-3 flex flex-col justify-between h-screen\"><div>")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = components.Header("Send").Render(ctx, templ_7745c5c3_Buffer)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("<p class=\"font-semibold mb-2\">Recipient's address</p><div class=\"bg-graybg p-4 flex items-center justify-between rounded-lg\"><input class=\"bg-graybg border-0\" id=\"sendAddress\" placeholder=\"ark1ccc...\"><p onclick=\"pasteAddressFromClipboard()\">")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = PasteIcon().Render(ctx, templ_7745c5c3_Buffer)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("</p></div><p class=\"font-semibold mb-2 mt-10\">Amount</p><div class=\"bg-graybg p-4 flex items-center justify-between rounded-lg\"><input class=\"bg-graybg border-0\" id=\"sendAmount\" max=\"")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		var templ_7745c5c3_Var4 string
		templ_7745c5c3_Var4, templ_7745c5c3_Err = templ.JoinStringErrs(currentBalance)
		if templ_7745c5c3_Err != nil {
			return templ.Error{Err: templ_7745c5c3_Err, FileName: `templates/pages/send.templ`, Line: 30, Col: 75}
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var4))
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("\" placeholder=\"5000\"><div class=\"flex items-center gap-4\"><p class=\"py-1 px-2 bg-offwhite/20 text-offwhite/50 rounded-lg\" onclick=\"setMaxValue()\">MAX</p><p class=\"py-1 w-10\" id=\"unit\">SATS</p><p onclick=\"toggleUnit()\">")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = ArrowDownIcon().Render(ctx, templ_7745c5c3_Buffer)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("</p></div></div><p class=\"text-right text-sm\" id=\"available\">Available <span class=\"sats\">")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		var templ_7745c5c3_Var5 string
		templ_7745c5c3_Var5, templ_7745c5c3_Err = templ.JoinStringErrs(currentBalance)
		if templ_7745c5c3_Err != nil {
			return templ.Error{Err: templ_7745c5c3_Err, FileName: `templates/pages/send.templ`, Line: 37, Col: 93}
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var5))
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("</span> SATS</p></div><div><button class=\"bg-orange mb-2\">Confirm</button> <button class=\"bg-graybg w-full\" onclick=\"redirect(&#39;/&#39;)\">Cancel</button></div></div><script>\n\t  const pasteAddressFromClipboard = () => {\n\t\t\tif (navigator.clipboard) {\n        navigator.clipboard.readText().then((val) => {\n\t\t\t\t\tdocument.querySelector('#sendAddress').value = val\n\t\t\t  })\n      }\n\t\t}\n\t\t\n\t\tconst setMaxValue = () => {\n\t\t\tconst sats = document.querySelector('#sendAmount').getAttribute('max')\n\t\t\tif (isNaN(sats)) return\n\t\t\tdocument.querySelector('#sendAmount').value = sats\n\t\t}\n\n\t\tconst toggleUnit = () => {\n\t\t\tconst currUnit = document.querySelector('#unit').innerText\n\t\t\tconst nextUnit = currUnit === 'SATS' ? 'BTC' : 'SATS'\n\t\t\t// change unit\n\t\t\tdocument.querySelector('#unit').innerText = nextUnit\n\t\t\t// change availability\n\t\t\tconst maxSats = document.querySelector('#sendAmount').getAttribute('max')\n\t\t\tdocument.querySelector('#available').innerText =\n\t\t\t  `Available ${nextUnit === 'SATS' ? prettyNum(maxSats) : fromSatoshis(maxSats)} ${nextUnit}`\n\t\t\t// change value inside input\n\t\t\tconst currVal = document.querySelector('#sendAmount').value\n\t\t\tif (currVal) {\n\t\t\t\tconst nextVal = nextUnit === 'SATS' ? toSatoshis(currVal) : fromSatoshis(currVal)\n\t\t\t\tdocument.querySelector('#sendAmount').value = nextVal\n\t\t\t}\n\t\t}\n\t</script>")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		return templ_7745c5c3_Err
	})
}
