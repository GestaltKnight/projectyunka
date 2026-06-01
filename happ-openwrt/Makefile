include $(TOPDIR)/rules.mk

PKG_NAME:=happ-openwrt
PKG_VERSION:=0.1.0
PKG_RELEASE:=1

PKG_BUILD_DIR:=$(BUILD_DIR)/$(PKG_NAME)-$(PKG_VERSION)

include $(INCLUDE_DIR)/package.mk
include $(TOPDIR)/feeds/packages/lang/golang/golang-package.mk

GO_PKG:=github.com/happ-openwrt/happ-openwrt
GO_PKG_BUILD_PKG:=$(GO_PKG)/cmd/happ-openwrt

define Package/happ-openwrt
  SECTION:=net
  CATEGORY:=Network
  TITLE:=Happ-compatible subscription importer for sing-box
  DEPENDS:=$(GO_ARCH_DEPENDS) +sing-box
endef

define Package/happ-openwrt/description
  Imports supported Happ/proxy subscription links and generates a sing-box
  configuration with TUN routing for OpenWrt.
endef

define Package/happ-openwrt/conffiles
/etc/config/happ-openwrt
endef

define Build/Prepare
	$(call Build/Prepare/Default)
	$(CP) ./cmd ./go.mod $(PKG_BUILD_DIR)/
endef

define Package/happ-openwrt/install
	$(call GoPackage/Package/Install/Bin,$(1))
	$(INSTALL_DIR) $(1)/etc/config
	$(INSTALL_CONF) ./files/etc/config/happ-openwrt $(1)/etc/config/happ-openwrt
	$(INSTALL_DIR) $(1)/etc/init.d
	$(INSTALL_BIN) ./files/etc/init.d/happ-openwrt $(1)/etc/init.d/happ-openwrt
endef

$(eval $(call GoBinPackage,happ-openwrt))
$(eval $(call BuildPackage,happ-openwrt))
