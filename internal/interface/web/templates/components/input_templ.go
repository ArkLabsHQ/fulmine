// Code generated by templ - DO NOT EDIT.

// templ: version: v0.2.747
package components

//lint:file-ignore SA4006 This context is only used if a nested component is present.

import "github.com/a-h/templ"
import templruntime "github.com/a-h/templ/runtime"

func SendAmount(max string) templ.Component {
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
		templ_7745c5c3_Err = InputAmount(max).Render(ctx, templ_7745c5c3_Buffer)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		return templ_7745c5c3_Err
	})
}

func ReceiveAmount() templ.Component {
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
		templ_7745c5c3_Err = InputAmount("").Render(ctx, templ_7745c5c3_Buffer)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		return templ_7745c5c3_Err
	})
}

func InputAmount(max string) templ.Component {
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
		templ_7745c5c3_Err = Label("Amount").Render(ctx, templ_7745c5c3_Buffer)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("<div class=\"bg-graybg p-4 flex items-center justify-between rounded-lg\"><input class=\"hidden\" id=\"sats\" name=\"sats\"> <input class=\"bg-graybg border-0 p-0\" id=\"amount\"")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		if len(max) > 0 {
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(" max=\"")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			var templ_7745c5c3_Var4 string
			templ_7745c5c3_Var4, templ_7745c5c3_Err = templ.JoinStringErrs(max)
			if templ_7745c5c3_Err != nil {
				return templ.Error{Err: templ_7745c5c3_Err, FileName: `internal/interface/web/templates/components/input.templ`, Line: 19, Col: 15}
			}
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var4))
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("\"")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(" onchange=\"updateSats()\" placeholder=\"5000\"><div class=\"flex items-center gap-2\">")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		if len(max) > 0 {
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("<p class=\"py-1 px-2 bg-white/20 text-white/50 rounded-lg cursor-pointer\" onclick=\"setMaxValue()\">MAX</p>")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("<p class=\"py-1 w-10 text-center\" id=\"unit\">SATS</p><p class=\"cursor-pointer\" onclick=\"toggleUnit()\">")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = ChevronDownIcon().Render(ctx, templ_7745c5c3_Buffer)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("</p></div></div>")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		if len(max) > 0 {
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("<p class=\"text-right text-sm\" id=\"available\">Available <span class=\"sats\">")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			var templ_7745c5c3_Var5 string
			templ_7745c5c3_Var5, templ_7745c5c3_Err = templ.JoinStringErrs(max)
			if templ_7745c5c3_Err != nil {
				return templ.Error{Err: templ_7745c5c3_Err, FileName: `internal/interface/web/templates/components/input.templ`, Line: 32, Col: 82}
			}
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString(templ.EscapeString(templ_7745c5c3_Var5))
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
			_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("</span> SATS</p>")
			if templ_7745c5c3_Err != nil {
				return templ_7745c5c3_Err
			}
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("<script>\n\t\tconst setMaxValue = () => {\n\t\t\tconst sats = document.querySelector('#amount').getAttribute('max')\n\t\t\tif (isNaN(sats)) return\n\t\t\tconst unit = document.querySelector('#unit').innerText\n\t\t\tdocument.querySelector('#amount').value = unit === 'SATS' ? sats : fromSatoshis(sats)\n\t\t\tdocument.querySelector('#sats').value = sats\n\t\t}\n\n\t\tconst toggleUnit = () => {\n\t\t\tconst currUnit = document.querySelector('#unit').innerText\n\t\t\tconst nextUnit = currUnit === 'SATS' ? 'BTC' : 'SATS'\n\t\t\t// change unit\n\t\t\tdocument.querySelector('#unit').innerText = nextUnit\n\t\t\t// change availability\n\t\t\tconst maxSats = document.querySelector('#amount').getAttribute('max')\n\t\t\tdocument.querySelector('#available').innerText =\n\t\t\t  `Available ${nextUnit === 'SATS' ? prettyNum(maxSats) : fromSatoshis(maxSats)} ${nextUnit}`\n\t\t\t// change value inside input\n\t\t\tconst currVal = document.querySelector('#amount').value\n\t\t\tif (currVal) {\n\t\t\t\tconst nextVal = nextUnit === 'SATS' ? toSatoshis(currVal) : fromSatoshis(currVal)\n\t\t\t\tdocument.querySelector('#amount').value = nextVal\n\t\t\t}\n\t\t}\n\n\t\tconst updateSats = () => {\n\t\t\tconst unit = document.querySelector('#unit').innerText\n\t\t\tconst amount = document.querySelector('#amount').value\n      document.querySelector('#sats').value = unit === 'SATS' ? amount : toSatoshis(amount)\n\t\t}\n\t</script>")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		return templ_7745c5c3_Err
	})
}

func InputAddress() templ.Component {
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
		templ_7745c5c3_Var6 := templ.GetChildren(ctx)
		if templ_7745c5c3_Var6 == nil {
			templ_7745c5c3_Var6 = templ.NopComponent
		}
		ctx = templ.ClearChildren(ctx)
		templ_7745c5c3_Err = Label("Address").Render(ctx, templ_7745c5c3_Buffer)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("<div class=\"bg-graybg p-4 flex items-center justify-between rounded-lg\"><input class=\"bg-graybg border-0 p-0\" id=\"address\" name=\"address\" placeholder=\"ark1ccc...\"><p class=\"cursor-pointer text-white/50\" onclick=\"pasteAddressFromClipboard()\">")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		templ_7745c5c3_Err = PasteIconXL().Render(ctx, templ_7745c5c3_Buffer)
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		_, templ_7745c5c3_Err = templ_7745c5c3_Buffer.WriteString("</p></div><script>\n\t  const pasteAddressFromClipboard = () => {\n\t\t\tif (navigator.clipboard) {\n        navigator.clipboard.readText().then((addr) => {\n\t\t\t\t\tdocument.querySelector('#address').value = addr\n\t\t\t\t\tconst asAmount = addr.match(/amount:(\\d+)/)\n\t\t\t\t\tif (asAmount) {\n\t\t\t\t\t\tconst sats = asAmount[1]\n\t\t\t\t\t\tconst unit = document.querySelector('#unit').innerText\n\t\t\t\t\t\tdocument.querySelector('#amount').value = unit === 'SATS' ? sats : fromSatoshis(sats)\n\t\t\t\t\t\tdocument.querySelector('#sats').value = sats\n\t\t\t\t\t}\n\t\t\t  })\n      }\n\t\t}\n\t</script>")
		if templ_7745c5c3_Err != nil {
			return templ_7745c5c3_Err
		}
		return templ_7745c5c3_Err
	})
}
