module("luci.controller.happ_openwrt", package.seeall)

function index()
	if not nixio.fs.access("/etc/config/happ-openwrt") then
		return
	end

	entry({"admin", "services", "happ-openwrt"}, cbi("happ_openwrt"), _("Happ OpenWrt"), 60).dependent = true
	entry({"admin", "services", "happ-openwrt", "update"}, call("action_update")).leaf = true
end

function action_update()
	luci.sys.call("/etc/init.d/happ-openwrt update-subscription >/dev/null 2>&1")
	luci.http.redirect(luci.dispatcher.build_url("admin", "services", "happ-openwrt"))
end
