include $(TOPDIR)/rules.mk

PKG_NAME:=happ-openwrt
PKG_VERSION:=0.1.0
PKG_RELEASE:=1

PKG_BUILD_DIR:=$(BUILD_DIR)/$(PKG_NAME)-$(PKG_VERSION)

include $(INCLUDE_DIR)/package.mk

define Package/happ-openwrt
  SECTION:=net
  CATEGORY:=Network
  TITLE:=Happ-compatible subscription importer for sing-box
  DEPENDS:=+sing-box +ca-bundle
endef

define Package/happ-openwrt/description
  Imports supported Happ/proxy subscription links and generates a sing-box configuration with TUN routing for OpenWrt.
endef

define Package/happ-openwrt/conffiles
/etc/config/happ-openwrt
endef

define Build/Prepare
	mkdir -p $(PKG_BUILD_DIR)
	$(CP) ./cmd $(PKG_BUILD_DIR)/ 2>/dev/null || true
	$(CP) ./internal $(PKG_BUILD_DIR)/ 2>/dev/null || true
	$(CP) ./pkg $(PKG_BUILD_DIR)/ 2>/dev/null || true
	$(CP) ./go.mod $(PKG_BUILD_DIR)/
	$(CP) ./go.sum $(PKG_BUILD_DIR)/ 2>/dev/null || true
endef

define Build/Compile
	cd $(PKG_BUILD_DIR) && \
		GOCACHE=$(TMP_DIR)/go-cache \
		GOOS=linux \
		GOARCH=arm64 \
		CGO_ENABLED=0 \
		go build -mod=mod -trimpath -ldflags="-s -w" -o happ-openwrt ./cmd/happ-openwrt
endef

define Package/happ-openwrt/install
	$(INSTALL_DIR) $(1)/usr/bin
	$(INSTALL_BIN) $(PKG_BUILD_DIR)/happ-openwrt $(1)/usr/bin/happ-openwrt

	$(INSTALL_DIR) $(1)/etc/config
	$(INSTALL_CONF) ./files/etc/config/happ-openwrt $(1)/etc/config/happ-openwrt

	$(INSTALL_DIR) $(1)/etc/init.d
	$(INSTALL_BIN) ./files/etc/init.d/happ-openwrt $(1)/etc/init.d/happ-openwrt
endef

$(eval $(call BuildPackage,happ-openwrt))
