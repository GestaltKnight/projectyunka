local m, s, o

m = Map("happ-openwrt", translate("Happ OpenWrt"))
m.description = translate("Import supported Happ/proxy subscriptions and generate a sing-box TUN configuration.")

s = m:section(NamedSection, "main", "happ-openwrt", translate("Settings"))
s.addremove = false

o = s:option(Flag, "enabled", translate("Enable"))
o.rmempty = false

o = s:option(Value, "subscription_url", translate("Subscription URL"))
o.password = true
o.rmempty = false

o = s:option(Value, "update_interval", translate("Update interval"))
o.datatype = "uinteger"
o.default = "24"
o.description = translate("Hours between subscription updates when scheduled externally.")

o = s:option(ListValue, "proxy_mode", translate("Proxy mode"))
o:value("global", translate("Global"))
o:value("rule", translate("Rule"))
o:value("direct", translate("Direct"))
o.default = "global"

o = s:option(ListValue, "dns_mode", translate("DNS mode"))
o:value("remote", translate("Remote"))
o:value("local", translate("Local"))
o:value("system", translate("System"))
o.default = "remote"

o = s:option(Value, "user_agent", translate("Custom User-Agent"))
o.rmempty = true

o = s:option(Value, "hwid", translate("HWID"))
o.password = true
o.rmempty = true

o = s:option(Value, "installid", translate("InstallID"))
o.password = true
o.rmempty = true

o = s:option(Button, "_update", translate("Update subscription"))
o.inputstyle = "apply"
function o.write()
	luci.http.redirect(luci.dispatcher.build_url("admin", "services", "happ-openwrt", "update"))
end

return m
