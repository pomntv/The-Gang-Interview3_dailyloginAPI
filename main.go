package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"hoyolab/act"

	"github.com/go-resty/resty/v2"
	"github.com/tmilewski/goenv"
	"github.com/zellyn/kooky"
	"github.com/zellyn/kooky/browser/chrome"
)

// at *day 1* your got [GSI]asdasd x1, [HSR]asdasd x1, [HI3]asdasd x1
var configExt string = "yaml"
var logExt string = "log"
var configPath string = ""
var logPath string = ""
var logfile *os.File

func init() {
	goenv.Load()
	IsDev := os.Getenv(act.DEBUG) != ""

	execFilename, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	baseFilename := strings.ReplaceAll(filepath.Base(execFilename), filepath.Ext(execFilename), "")
	dirname := filepath.Dir(execFilename)

	if IsDev {
		dirname, err = os.UserHomeDir()
		if err != nil {
			log.Fatal(err)
		}
	}

	configPath = path.Join(dirname, fmt.Sprintf("%s.%s", baseFilename, configExt))
	logPath = path.Join(dirname, fmt.Sprintf("%s.%s", baseFilename, logExt))

	log.SetFlags(log.Lshortfile | log.Ltime)
	if !IsDev {
		log.SetFlags(log.Ldate | log.Ltime)
		f, err := os.OpenFile(logPath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			log.Fatal(err)
		}
		log.SetOutput(f)
	}
}

func main() {
	if logfile != nil {
		defer logfile.Close()
	}

	hoyo := GenerateDefaultConfig()
	if err := hoyo.ReadHoyoConfig(configPath); err != nil {
		log.Fatal(err)
	}

	cookieStore := kooky.FindAllCookieStores()
	fmt.Printf("Browser total %d sessions \n", len(cookieStore))
	var notifyMessage []string
	for _, store := range cookieStore {
		if _, err := os.Stat(store.FilePath()); os.IsNotExist(err) {
			continue
		}

		for _, profile := range hoyo.Browser {
			if len(profile.Name) > 0 && (store.Browser() != profile.Browser || !ContainsStrings(profile.Name, store.Profile())) {
				continue
			}
			fmt.Printf(`Profile %s::"%s" \n`, store.Browser(), store.Profile())

			cookies, err := chrome.CookieJar(store.FilePath())
			if err != nil {
				log.Fatal(err)
			}

			if !hoyo.IsCookieToken(cookies) {
				log.Println("Session is empty, please login hoyolab.com.")
				continue
			}

			var resAcc *act.ActUser
			hoyo.Client = resty.New()

			var getDaySign int32 = -1
			var getAward []string = []string{}
			for i := 0; i < len(hoyo.Daily); i++ {
				act := hoyo.Daily[i]
				act.UserAgent = profile.UserAgent

				if resAcc == nil {
					resAcc, err = act.GetAccountUserInfo(hoyo)
					if err != nil {
						fmt.Printf("%s::GetUserInfo    : %v \n", act.Label, err)
						continue
					}
					fmt.Printf("%s::GetUserInfo    : Hi, '%s \n", act.Label, resAcc.UserInfo.NickName)
				}

				resAward, err := act.GetMonthAward(hoyo)
				if err != nil {
					fmt.Printf("%s::GetMonthAward  : %v \n", act.Label, err)
					continue
				}

				resInfo, err := act.GetCheckInInfo(hoyo)
				if err != nil {
					fmt.Printf("%s::GetCheckInInfo :%v \n", act.Label, err)
					continue
				}

				fmt.Printf("%s::GetCheckInInfo : Checked in for %d days \n", act.Label, resInfo.TotalSignDay)
				if resInfo.IsSign {
					fmt.Printf("%s::DailySignIn    : Claimed %s \n", act.Label, resInfo.Today)
					continue
				}

				_, err = act.DailySignIn(hoyo)
				if err != nil {
					fmt.Printf("%s::DailySignIn    : %v \n", act.Label, err)
					continue
				}

				if getDaySign < 0 {
					getDaySign = resInfo.TotalSignDay + 1
				}

				award := resAward.Awards[resInfo.TotalSignDay+1]
				fmt.Printf("%s::GetMonthAward  : Today's received %s x%d \n", act.Label, award.Name, award.Count)

				if hoyo.Notify.Mini {
					getAward = append(getAward, fmt.Sprintf("*%s x%d* (%s)", award.Name, award.Count, act.Label))
				} else {
					getAward = append(getAward, fmt.Sprintf("*[%s]* at day %d received %s x%d", act.Label, resInfo.TotalSignDay+1, award.Name, award.Count))
				}

			}
			if len(getAward) > 0 {
				if len(hoyo.Browser) > 1 {
					notifyMessage = append(notifyMessage, "\n")
				}

				if hoyo.Notify.Mini {
					notifyMessage = append(notifyMessage, fmt.Sprintf("%s, at day %d your got %s", resAcc.UserInfo.NickName, getDaySign, strings.Join(getAward, ", ")))
				} else {
					notifyMessage = append(notifyMessage, fmt.Sprintf("\nHi, %s Checked in for %d days.\n%s", resAcc.UserInfo.NickName, 1, strings.Join(getAward, "\n")))
				}
			}
		}
	}

	if err := hoyo.NotifyMessage(strings.Join(notifyMessage, "\n")); err != nil {
		fmt.Printf("NotifyMessage  : %v \n", err)
	}
}

func ContainsStrings(a []string, x string) bool {
	sort.Strings(a)
	for _, v := range a {
		if v == x {
			return true
		}
	}
	return false
}

func GenerateDefaultConfig() *act.Hoyolab {
	// Genshin Impact
	//
	// {"code":"ok"}
	// {"retcode":0,"message":"OK","data":{"total_sign_day":11,"today":"2022-10-29","is_sign":false,"first_bind":false,"is_sub":true,"region":"os_asia","month_last_day":false}}
	// {"retcode":0,"message":"OK","data":{"code":"ok","first_bind":false,"gt_result":{"risk_code":0,"gt":"","challenge":"","success":0,"is_risk":false}}}
	var apiGenshinImpact = &act.DailyHoyolab{
		CookieJar: []*http.Cookie{},
		Label:     "GSI",
		ActID:     "e202102251931481",
		API: act.DailyAPI{
			Endpoint: "https://sg-hk4e-api.hoyolab.com",
			Domain:   "https://hoyolab.com",
			Award:    "/event/sol/home",
			Info:     "/event/sol/info",
			Sign:     "/event/sol/sign",
		},
		Lang:    "en-us",
		Referer: "https://act.hoyolab.com/ys/event/signin-sea-v3/index.html",
	}

	// Honkai StarRail
	//
	// https://sg-public-api.hoyolab.com/event/luna/os/home?lang=en-us&act_id=e202303301540311
	// https://sg-public-api.hoyolab.com/event/luna/os/sign
	// {"retcode":0,"message":"OK","data":{"code":"","risk_code":0,"gt":"","challenge":"","success":0,"is_risk":false}}
	// https://sg-public-api.hoyolab.com/event/luna/os/info
	// {"retcode":0,"message":"OK","data":{"total_sign_day":7,"today":"2023-05-09","is_sign":false,"is_sub":false,"region":"","sign_cnt_missed":1,"short_sign_day":0}}
	//
	// var apiHonkaiStarRail = &act.DailyHoyolab{
	// 	CookieJar: []*http.Cookie{},
	// 	Label:     "HRS",
	// 	ActID:     "e202303301540311",
	// 	API: act.DailyAPI{
	// 		Endpoint: "https://sg-public-api.hoyolab.com",
	// 		Domain:   "https://hoyolab.com",
	// 		Award:    "/event/luna/os/home",
	// 		Info:     "/event/luna/os/info",
	// 		Sign:     "/event/luna/os/sign",
	// 	},
	// 	Lang:    "en-us",
	// 	Referer: "https://act.hoyolab.com/bbs/event/signin/hkrpg/index.html",
	// }

	// Honkai Impact 3
	// https: //act.hoyolab.com/bbs/event/signin-bh3/index.html?act_id=e202110291205111
	// var apiHonkaiImpact = &act.DailyHoyolab{
	// 	CookieJar: []*http.Cookie{},
	// 	Label:     "HI3",
	// 	ActID:     "e202110291205111",
	// 	API: act.DailyAPI{
	// 		Endpoint: "https://sg-public-api.hoyolab.com",
	// 		Domain:   "https://hoyolab.com",
	// 		Award:    "/event/mani/home",
	// 		Sign:     "/event/mani/sign",
	// 		Info:     "/event/mani/info",
	// 	},
	// 	Lang:    "en-us",
	// 	Referer: "https://act.hoyolab.com/bbs/event/signin-bh3/index.html",
	// }
	return &act.Hoyolab{
		Notify: act.LineNotify{
			Token: "",
			Mini:  true,
		},
		Delay: 150,
		Browser: []act.BrowserProfile{
			{
				Browser:   "chrome",
				Name:      []string{},
				UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/113.0.0.0 Safari/537.36",
			},
		},
		Daily: []*act.DailyHoyolab{
			apiGenshinImpact,
			// apiHonkaiStarRail,
			// apiHonkaiImpact,
		},
	}
}
