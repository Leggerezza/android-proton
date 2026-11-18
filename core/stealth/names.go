package stealth

import (
	"hash/crc32"
	"net"
)

// Real, popular hostnames used as TLS SNI for the Stealth handshake. The list
// is Proton's (ProtonVPN/wireguard-go conn/server_name_utils) so that the
// handshake looks like ordinary HTTPS to a middlebox.
var serverNames = []string{
	"accounts.google.com",
	"activity.windows.com",
	"analytics.apis.mcafee.com",
	"android.apis.google.com",
	"android.googleapis.com",
	"api.account.samsung.com",
	"api.accounts.firefox.com",
	"api.accuweather.com",
	"api.amazon.com",
	"api.browser.yandex.net",
	"api.ipify.org",
	"api.onedrive.com",
	"api.reasonsecurity.com",
	"api.samsungcloud.com",
	"api.sec.intl.miui.com",
	"api.vk.com",
	"api.weather.com",
	"app-site-association.cdn-apple.com",
	"apps.mzstatic.com",
	"assets.msn.com",
	"backup.googleapis.com",
	"brave-core-ext.s3.brave.com",
	"caldav.calendar.yahoo.com",
	"cc-api-data.adobe.io",
	"cdn.ampproject.org",
	"cdn.cookielaw.org",
	"client.wns.windows.com",
	"cloudflare.com",
	"cloudflare-dns.com",
	"cloudflare-ech.com",
	"config.extension.grammarly.com",
	"connectivitycheck.android.com",
	"connectivitycheck.gstatic.com",
	"courier.push.apple.com",
	"crl.globalsign.com",
	"dc1-file.ksn.kaspersky-labs.com",
	"dl.google.com",
	"dns.google",
	"dns.quad9.net",
	"doh.cleanbrowsing.org",
	"doh.dns.apple.com",
	"doh.opendns.com",
	"doh.pub",
	"ds.kaspersky.com",
	"ecs.office.com",
	"edge.microsoft.com",
	"events.gfe.nvidia.com",
	"excess.duolingo.com",
	"firefox.settings.services.mozilla.com",
	"fonts.googleapis.com",
	"fonts.gstatic.com",
	"gateway-asset.icloud-content.com",
	"gateway.icloud.com",
	"gdmf.apple.com",
	"github.com",
	"go.microsoft.com",
	"go-updater.brave.com",
	"graph.microsoft.com",
	"gs-loc.apple.com",
	"gtglobal.intl.miui.com",
	"hcaptcha.com",
	"imap.gmail.com",
	"imap-mail.outlook.com",
	"imap.mail.yahoo.com",
	"in.appcenter.ms",
	"ipmcdn.avast.com",
	"itunes.apple.com",
	"loc.map.baidu.com",
	"login.live.com",
	"login.microsoftonline.com",
	"m.media-amazon.com",
	"mobile.events.data.microsoft.com",
	"mozilla.cloudflare-dns.com",
	"mtalk.google.com",
	"nimbus.bitdefender.net",
	"ocsp2.apple.com",
	"outlook.office365.com",
	"play-fe.googleapis.com",
	"play.googleapis.com",
	"play.samsungcloud.com",
	"raw.githubusercontent.com",
	"s3.amazonaws.com",
	"safebrowsing.googleapis.com",
	"s.alicdn.com",
	"self.events.data.microsoft.com",
	"settings-win.data.microsoft.com",
	"setup.icloud.com",
	"sirius.mwbsys.com",
	"spoc.norton.com",
	"ssl.gstatic.com",
	"unpkg.com",
	"update.googleapis.com",
	"weatherapi.intl.xiaomi.com",
	"weatherkit.apple.com",
	"westus-0.in.applicationinsights.azure.com",
	"www.googleapis.com",
	"www.gstatic.com",
	"www.msftconnecttest.com",
	"www.msftncsi.com",
	"www.ntppool.org",
	"www.pool.ntp.org",
	"www.recaptcha.net",
}

// ServerNameForIP picks a stable SNI for an entry IP, so reconnects to the
// same server keep the same (plausible) hostname.
func ServerNameForIP(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return serverNames[0]
	}
	return serverNames[int(crc32.ChecksumIEEE([]byte(ip)))%len(serverNames)]
}

// ChinaSafeServerNames is the subset of Proton's SNI pool that is not on the
// GFW's TLS-SNI blacklist. DoH endpoints (cloudflare-dns.com, dns.google,
// doh.*), raw.githubusercontent.com and a few others are reset on sight in
// mainland China, so picking them from the full pool makes the handshake fail
// with a TCP reset even though the entry IP itself is reachable.
var ChinaSafeServerNames = []string{
	"gateway.icloud.com",
	"www.ntppool.org",
	"www.pool.ntp.org",
	"connectivitycheck.gstatic.com",
	"connectivitycheck.android.com",
	"dl.google.com",
	"ssl.gstatic.com",
	"www.gstatic.com",
	"fonts.gstatic.com",
	"update.googleapis.com",
	"play.googleapis.com",
	"android.googleapis.com",
	"api.amazon.com",
	"s3.amazonaws.com",
	"login.live.com",
	"graph.microsoft.com",
	"edge.microsoft.com",
	"outlook.office365.com",
	"www.msftconnecttest.com",
	"www.msftncsi.com",
	"courier.push.apple.com",
	"gs-loc.apple.com",
}

// SafeServerNamesForIP returns the China-safe SNI pool rotated so that a
// different (but still plausible) name is tried first for each new
// connection attempt. Proton's Stealth terminator accepts any name from its
// own pool, so rotation costs nothing and gets past a blocked name.
func SafeServerNamesForIP(ip string) []string {
	if len(ChinaSafeServerNames) == 0 {
		return nil
	}
	start := int(crc32.ChecksumIEEE([]byte(ip))) % len(ChinaSafeServerNames)
	out := make([]string, 0, len(ChinaSafeServerNames))
	for i := 0; i < len(ChinaSafeServerNames); i++ {
		out = append(out, ChinaSafeServerNames[(start+i)%len(ChinaSafeServerNames)])
	}
	return out
}
