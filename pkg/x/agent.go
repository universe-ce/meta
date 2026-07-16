package x

import (
	"fmt"
	"github.com/kataras/iris/v12"
	"net"
	"net/http"
	"regexp"
	"strings"
)

func GetClientIp(c iris.Context) string {
	ip := ClientPublicIP(c.Request())
	if ip == "" {
		ip = ClientIP(c.Request())
	}
	return ip
}

// ClientIP 尽最大努力实现获取客户端 IP 的算法。
// 解析 X-Real-IP 和 X-Forwarded-For 以便于反向代理（nginx 或 haproxy）可以正常工作。
func ClientIP(r *http.Request) string {
	xForwardedFor := r.Header.Get("X-Forwarded-For")
	ip := strings.TrimSpace(strings.Split(xForwardedFor, ",")[0])
	if ip != "" {
		return ip
	}
	ip = strings.TrimSpace(r.Header.Get("X-Real-Ip"))
	if ip != "" {
		return ip
	}
	if ip, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr)); err == nil {
		return ip
	}
	return ""
}

// ClientPublicIP 尽最大努力实现获取客户端公网 IP 的算法。
// 解析 X-Real-IP 和 X-Forwarded-For 以便于反向代理（nginx 或 haproxy）可以正常工作。
func ClientPublicIP(r *http.Request) string {
	var ip string
	for _, ip = range strings.Split(r.Header.Get("X-Forwarded-For"), ",") {
		ip = strings.TrimSpace(ip)
		if ip != "" && !HasLocalIPAddr(ip) {
			return ip
		}
	}
	ip = strings.TrimSpace(r.Header.Get("X-Real-Ip"))
	if ip != "" && !HasLocalIPAddr(ip) {
		return ip
	}
	if ip, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr)); err == nil {
		if !HasLocalIPAddr(ip) {
			return ip
		}
	}
	return ""
}

// HasLocalIPAddr 检测 IP 地址字符串是否是内网地址
func HasLocalIPAddr(ip string) bool {
	return HasLocalIP(net.ParseIP(ip))
}

func HasLocalIP(ip net.IP) bool {
	if ip.IsLoopback() {
		return true
	}

	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}

	return ip4[0] == 10 || // 10.0.0.0/8
		(ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31) || // 172.16.0.0/12
		(ip4[0] == 169 && ip4[1] == 254) || // 169.254.0.0/16
		(ip4[0] == 192 && ip4[1] == 168) // 192.168.0.0/16
}

/**
 * @Description: Get WinNT name at Window NT
 * @param sName
 * @return string
 */
func getWinOsNameWithWinNT(sName string) string {
	osName := "Windows"
	types := map[string]string{
		"Windows NT 10":  "Windows 10",
		"Windows NT 6.3": "Windows 8",
		"Windows NT 6.2": "Windows 8",
		"Windows NT 6.1": "Windows 7",
		"Windows NT 6.0": "Windows Vista/Server 2008",
		"Windows NT 5.2": "Windows Server 2003",
		"Windows NT 5.1": "Windows XP",
		"Windows NT 5":   "Windows 2000",
		"Windows NT 4":   "Windows NT4",
	}

	for keyWord, name := range types {
		if strings.Contains(sName, keyWord) {
			osName = name
			break
		}
	}

	return osName
}

// GetOsName 获取OS名称
func GetOsName(userAgent string) string {
	osName := "Unknown"
	if userAgent == "" {
		return osName
	}

	strRe, _ := regexp.Compile("(?i:\\((.*?)\\))")
	userAgent = strRe.FindString(userAgent)

	levelNames := ":micromessenger:dart:Windows NT:Windows Mobile:Windows Phone:Windows Phone OS:Macintosh|Macintosh:Mac OS:CrOS|CrOS:iPhone OS:iPad|iPad:OS:Android:Linux:blackberry:hpwOS:Series:Symbian:PalmOS:SymbianOS:J2ME:Sailfish:Bada:MeeGo:webOS|hpwOS:Maemo:"
	var regStrArr []string
	namesArr := strings.Split(strings.Trim(levelNames, ":"), ":")
	for _, name := range namesArr {
		regStrArr = append(regStrArr, fmt.Sprintf("(%s[\\s?\\/XxSs0-9_.]+)", name))
	}
	regexpStr := fmt.Sprintf("(?i:%s)", strings.Join(regStrArr, "|"))
	nameRe, _ := regexp.Compile(regexpStr)

	names := nameRe.FindAllString(userAgent, -1)
	name := ""
	for _, s := range names {
		if name == "" {
			name = strings.TrimSpace(s)
		} else if len(name) > 0 {
			if strings.Contains(name, "Macintosh") && s != "" {
				name = strings.TrimSpace(s)
			} else if strings.Contains(name, s) {
				name = strings.TrimSpace(s)
			} else if !strings.Contains(s, name) {
				if strings.Contains(name, "iPhone") ||
					strings.Contains(name, "iPad") {
					s = strings.Trim(s, "Mac OS X")
				}

				if s != "" {
					name += " " + strings.TrimSpace(s)
				}
			}
			break
		}

		if strings.Contains(name, "Windows NT") {
			name = getWinOsNameWithWinNT(name)
			break
		}
	}

	if name != "" {
		osName = name
	}

	return osName
}

// GetBrowserName 获取浏览器名称
func GetBrowserName(userAgent string) string {
	deviceName := "Unknown"

	levelNames := ":VivoBrowser:QQDownload:QQBrowser:QQ:MQQBrowser:MicroMessenger:TencentTraveler:LBBROWSER:TaoBrowser:BrowserNG:UCWEB:TwonkyBeamBrowser:NokiaBrowser:OviBrowser:NF-Browser:OneBrowser:Obigo:DiigoBrowser:baidubrowser:baiduboxapp:xiaomi:Redmi:MI:Lumia:Micromax:MSIEMobile:IEMobile:EdgiOS:Yandex:Mercury:Openwave:TouchPad:UBrowser:Presto:Maxthon:MetaSr:Trident:Opera:IEMobile:Edge:Chrome:Chromium:OPR:CriOS:Firefox:FxiOS:fennec:CrMo:Safari:Nexus One:Nexus S:Nexus:Blazer:teashark:bolt:HTC:Dell:Motorola:Samsung:LG:Sony:SonyST:SonyLT:SonyEricsson:Asus:Palm:Vertu:Pantech:Fly:Wiko:i-mobile:Alcatel:Nintendo:Amoi:INQ:ONEPLUS:Tapatalk:PDA:Novarra-Vision:NetFront:Minimo:FlyFlow:Dolfin:Nokia:Series:AppleWebKit:Mobile:Mozilla:Version:"

	var regStrArr []string
	namesArr := strings.Split(strings.Trim(levelNames, ":"), ":")
	for _, name := range namesArr {
		regStrArr = append(regStrArr, fmt.Sprintf("(%s[\\s?\\/0-9.]+)", name))
	}
	regexpStr := fmt.Sprintf("(?i:%s)", strings.Join(regStrArr, "|"))
	nameRe, _ := regexp.Compile(regexpStr)
	names := nameRe.FindAllString(userAgent, -1)

	level := 0
	for _, name := range names {
		replaceRe, _ := regexp.Compile("(?i:[\\s?\\/0-9.]+)")
		n := replaceRe.ReplaceAllString(name, "")
		l := strings.Index(levelNames, fmt.Sprintf(":%s:", n))
		if level == 0 {
			deviceName = strings.TrimSpace(name)
		}

		if l >= 0 && (level == 0 || level > l) {
			level = l
			deviceName = strings.TrimSpace(name)
		}
	}

	return deviceName
}

const (
	Android     = "android"
	Mobile      = "mobile"
	Ipad        = "ipad"
	Iphone      = "iphone"
	Ipod        = "ipod"
	Ios         = "ios"
	Kindle      = "Kindle"
	Silk        = "Silk"
	Postman     = "postman"
	Unknown     = "Unknown"
	Wap         = "wap"
	XwapProfile = "X-Wap-Profile"
	Profile     = "Profile"
)

var MobileUserAgentPrefixes = []string{
	"w3c ", "w3c-", "acs-", "alav", "alca", "amoi", "audi", "avan", "benq",
	"bird", "blac", "blaz", "brew", "cell", "cldc", "cmd-", "dang", "doco",
	"eric", "hipt", "htc_", "inno", "ipaq", "ipod", "jigs", "kddi", "keji",
	"leno", "lg-c", "lg-d", "lg-g", "lge-", "lg/u", "maui", "maxo", "midp",
	"mits", "mmef", "mobi", "mot-", "moto", "mwbp", "nec-", "newt", "noki",
	"palm", "pana", "pant", "phil", "play", "port", "prox", "qwap", "sage",
	"sams", "sany", "sch-", "sec-", "send", "seri", "sgh-", "shar", "sie-",
	"siem", "smal", "smar", "sony", "sph-", "symb", "t-mo", "teli", "tim-",
	"tosh", "tsm-", "upg1", "upsi", "vk-v", "voda", "wap-", "wapa", "wapi",
	"wapp", "wapr", "webc", "winw", "winw", "xda ", "xda-"}

var MobileUserAgentKeywords = []string{
	"blackberry", "webos", "ipod", "lge vx", "midp", "maemo", "mmp", "mobile",
	"netfront", "hiptop", "nintendo DS", "novarra", "openweb", "opera mobi",
	"opera mini", "palm", "psp", "phone", "smartphone", "symbian", "up.browser",
	"up.link", "wap", "windows ce"}

var TabletUserAgentKeywords = []string{"ipad", "playbook", "hp-tablet", "kindle"}

type device struct {
	normal   bool
	mobile   bool
	tablet   bool
	platform string
}

type Device interface {
	Normal() bool
	Mobile() bool
	Tablet() bool
	DeviceName() string
	Platform() string
}

// Middleware function that parses the User-Agent and other Header properties to determine
// the type of device being used.
//func Resolver() iris.Handler {
//	return func(c iris.Context) {
//		d := resolveDevice(c.Request())
//
//		c.Next()
//	}
//}

// ResolveDevice Reads the Header from a Request and attempts to determine what type of device the user is using.
// Utilizes various checks using the User-Agent,
func ResolveDevice(request *http.Request) Device {
	headers := make(map[string]string)
	//net/http中获取headers
	if len(request.Header) > 0 {
		for k, v := range request.Header {
			headers[k] = v[0]
			//fmt.Printf("%s=%s\n", k, v[0])
		}
	}
	agent := strings.ToLower(headers["User-Agent"])

	// Check Tablet
	if agent != "" {
		switch {
		case strings.Contains(agent, Android) && !strings.Contains(agent, Mobile):
			return &device{tablet: true, platform: Android}
		case strings.Contains(agent, Ipad):
			return &device{tablet: true, platform: Ipad}
		case strings.Contains(agent, Silk) && !strings.Contains(agent, Mobile):
			return &device{tablet: true, platform: Kindle}
		default:
			for _, keyword := range TabletUserAgentKeywords {
				if strings.Contains(agent, keyword) {
					return &device{tablet: true, platform: Unknown}
				}
			}
		}
	}

	// User Agent Profile detection
	xWapProfile := headers[XwapProfile]
	profile := headers[Profile]

	if xWapProfile != "" || profile != "" {
		if agent != "" {
			switch {
			case strings.Contains(agent, Android):
				return &device{mobile: true, platform: Android}
			case strings.Contains(agent, Iphone) || strings.Contains(agent, Ipod) || strings.Contains(agent, Ipad):
				return &device{mobile: true, platform: Ios}
			default:
				return &device{mobile: true, platform: Unknown}
			}
		}
	}

	// User Agent Prefix check
	if agent != "" && len(agent) >= 4 {
		prefix := agent[:4]
		for _, uaprefix := range MobileUserAgentPrefixes {
			if strings.Contains(prefix, uaprefix) {
				return &device{mobile: true, platform: Unknown}
			}
		}
	}

	// Accept Header check
	accept := headers["Accept"]
	if accept != "" && strings.Contains(accept, Wap) {
		return &device{mobile: true, platform: Unknown}
	}

	// Check Mobile
	if agent != "" {
		switch {
		case strings.Contains(agent, Android):
			return &device{mobile: true, platform: Android}
		case strings.Contains(agent, Iphone) || strings.Contains(agent, Ipod) || strings.Contains(agent, Ipad):
			return &device{mobile: true, platform: Ios}
		default:
			for _, keyword := range MobileUserAgentKeywords {
				if strings.Contains(agent, keyword) {
					return &device{mobile: true, platform: Unknown}
				}
			}
		}
	}

	// Check debug tools
	if agent != "" {
		switch {
		case strings.Contains(agent, Postman):
			return &device{normal: true, platform: Postman}
		default:
			// todo add some debug tools
		}
	}

	// Assume 'normal' if mobile or tablet was not identified
	return &device{normal: true, platform: agent}
}

// Shortcut for retrieving a Device object within a handler
//func GetDevice(c iris.Context) Device {
//	return c..(Device)
//}

// Normal Returns true if a device is Normal. Normal meaning not a Mobile or Tablet.
func (d *device) Normal() bool {
	return d.normal
}

// Mobile Returns true if a device is a mobile device
func (d *device) Mobile() bool {
	return d.mobile
}

// Tablet Returns true if a device is a tablet
func (d *device) Tablet() bool {
	return d.tablet
}

func (d *device) DeviceName() string {
	if d.Tablet() {
		return "tablet"
	}
	if d.Mobile() {
		return "mobile"
	}
	return "normal"
}

// Platform Returns the device platform. Possible values include IOS, Android, Kindle or Unknown
func (d *device) Platform() string {
	return d.platform
}

// GetClientDevice 获取客户端设备类型
//func GetClientDevice(r *http.Request) string {
//	//r * http.Request
//	var device string = "normal" // normal, mobile, tablet
//	//fmt.Println("打印Header参数列表：")
//	//put headers in a map
//	headers := make(map[string]string)
//	//net/http中获取headers
//	if len(r.Header) > 0 {
//		for k, v := range r.Header {
//			headers[k] = v[0]
//			//fmt.Printf("%s=%s\n", k, v[0])
//		}
//	}
//	via := strings.ToLower(headers["VIA"])
//	accept := strings.ToUpper(headers["Accept"])
//	HttpXWapProfile := headers["X_WAP_PROFILE"]
//	HttpProfile := headers["PROFILE"]
//	HttpUserAgent := headers["User-Agent"]
//	if via != "" && strings.Index(via, "wap") != -1 {
//		device = "mobile"
//	} else if accept != "" && strings.Index(accept, "VND.WAP.WML") != -1 {
//		device = "mobile"
//	} else if HttpXWapProfile != "" || HttpProfile != "" {
//		device = "mobile"
//	} else if HttpUserAgent != "" {
//		reg := regexp.MustCompile(`(?i:(blackberry|configuration\/cldc|hp |hp-|htc |htc_|htc-|iemobile|kindle|midp|mmp|motorola|mobile|nokia|opera mini|opera |Googlebot-Mobile|YahooSeeker\/M1A1-R2D2|android|iphone|ipod|mobi|palm|palmos|pocket|portalmmm|ppc;|smartphone|sonyericsson|sqh|spv|symbian|treo|up.browser|up.link|vodafone|windows ce|xda |xda_|MicroMessenger))`)
//		//fmt.Printf("%q\n", reg.FindAllString(HttpUserAgent, -1))
//		if len(reg.FindAllString(HttpUserAgent, -1)) > 0 {
//			device = "mobile"
//		}
//	}
//	return device
//}
