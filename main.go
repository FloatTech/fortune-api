package main

import (
	"archive/zip"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/fogleman/gg" // 注册了 jpg png gif
	"github.com/sirupsen/logrus"

	"github.com/FloatTech/zbputils/binary"
	"github.com/FloatTech/zbputils/file"
	"github.com/FloatTech/zbputils/img/pool"
	"github.com/FloatTech/zbputils/img/writer"
	"github.com/FloatTech/zbputils/math"
	"github.com/FloatTech/zbputils/process"
)

const (
	// 底图缓存位置
	images = "data/Fortune/"
	// 基础文件位置
	omikujson = "data/Fortune/text.json"
	// 字体文件夹位置
	fontdir = "data/Font/"
	// 字体文件位置
	font = fontdir + "sakura.ttf"
	// 生成图缓存位置
	cache = images + "cache/"
)

var (
	// 签文
	omikujis []map[string]string
)

func init() {
	_ = os.RemoveAll(cache)
	err := os.MkdirAll(cache, 0755)
	if err != nil {
		panic(err)
	}
	go func() {
		data, err := file.GetLazyData(omikujson, true, false)
		if err != nil {
			panic(err)
		}
		err = json.Unmarshal(data, &omikujis)
		if err != nil {
			panic(err)
		}
	}()
	go func() {
		_ = os.MkdirAll(fontdir, 0755)
		_, err := file.GetLazyData(font, false, true)
		if err != nil {
			panic(err)
		}
	}()
}

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage:", os.Args[0], "ip:port")
		return
	}
	http.HandleFunc("/fortune", handler)
	process.GlobalInitMutex.Unlock()
	logrus.Error(http.ListenAndServe(os.Args[1], nil))
}

func handler(resp http.ResponseWriter, req *http.Request) {
	// 检查是否GET请求
	if !methodis("GET", resp, req) {
		http.Error(resp, "405 Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	q := req.URL.Query()

	ids, ok := q["id"]
	if !ok {
		http.Error(resp, "400 BAD REQUEST: please specify int param id", http.StatusBadRequest)
		return
	}
	id, err := strconv.ParseInt(ids[0], 10, 64)
	if err != nil {
		http.Error(resp, "400 BAD REQUEST: "+err.Error(), http.StatusBadRequest)
		return
	}

	// 获取背景类型，默认车万
	kind := "车万"
	k, ok := q["kind"]
	if ok && k[0] != "" {
		kind, err = url.QueryUnescape(k[0])
		if err != nil {
			http.Error(resp, "400 BAD REQUEST: "+err.Error(), http.StatusBadRequest)
			return
		}
	}
	logrus.Debugln("[fortune]kind:", kind)

	// 检查背景图片是否存在
	zipfile := images + kind + ".zip"
	_, err = file.GetLazyData(zipfile, false, false)
	if err != nil {
		http.Error(resp, "500 Internal Server Error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 生成种子
	t, err := strconv.ParseInt(time.Now().Format("20060102"), 10, 64)
	if err != nil {
		http.Error(resp, "500 Internal Server Error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	seed := id + t

	// 随机获取背景
	background, index, err := randimage(zipfile, seed)
	if err != nil {
		http.Error(resp, "500 Internal Server Error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 随机获取签文
	title, text := randtext(seed)
	digest := md5.Sum(binary.StringToBytes(zipfile + strconv.Itoa(index) + title + text))
	cachefile := cache + hex.EncodeToString(digest[:])

	m, err := pool.GetImage(cachefile)
	if err == nil {
		http.Redirect(resp, req, m.String(), http.StatusTemporaryRedirect)
		return
	}
	if file.IsNotExist(cachefile) {
		f, err := os.Create(cachefile)
		if err != nil {
			http.Error(resp, "500 Internal Server Error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		_, err = draw(background, title, text, f)
		_ = f.Close()
	}
	http.ServeFile(resp, req, cachefile)
}

// @function randimage 随机选取zip内的文件
// @param path zip路径
// @param seed 随机数种子
// @return 文件路径 & 错误信息
func randimage(path string, seed int64) (im image.Image, index int, err error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return
	}
	defer reader.Close()

	r := rand.New(rand.NewSource(seed))
	index = r.Intn(len(reader.File))
	file := reader.File[index]
	f, err := file.Open()
	if err != nil {
		return
	}
	defer f.Close()

	im, _, err = image.Decode(f)
	return
}

// @function randtext 随机选取签文
// @param file 文件路径
// @param seed 随机数种子
// @return 签名 & 签文 & 错误信息
func randtext(seed int64) (string, string) {
	r := rand.New(rand.NewSource(seed))
	i := r.Intn(len(omikujis))
	return omikujis[i]["title"], omikujis[i]["content"]
}

// @function draw 绘制运势图
// @param background 背景图片路径
// @param seed 随机数种子
// @param title 签名
// @param text 签文
// @return 错误信息
func draw(back image.Image, title, txt string, f io.Writer) (int64, error) {
	canvas := gg.NewContext(back.Bounds().Size().Y, back.Bounds().Size().X)
	canvas.DrawImage(back, 0, 0)
	// 写标题
	canvas.SetRGB(1, 1, 1)
	if err := canvas.LoadFontFace(font, 45); err != nil {
		return -1, err
	}
	sw, _ := canvas.MeasureString(title)
	canvas.DrawString(title, 140-sw/2, 112)
	// 写正文
	canvas.SetRGB(0, 0, 0)
	if err := canvas.LoadFontFace(font, 23); err != nil {
		return -1, err
	}
	tw, th := canvas.MeasureString("测")
	tw, th = tw+10, th+10
	r := []rune(txt)
	xsum := rowsnum(len(r), 9)
	switch xsum {
	default:
		for i, o := range r {
			xnow := rowsnum(i+1, 9)
			ysum := math.Min(len(r)-(xnow-1)*9, 9)
			ynow := i%9 + 1
			canvas.DrawString(string(o), -offest(xsum, xnow, tw)+115, offest(ysum, ynow, th)+320.0)
		}
	case 2:
		div := rowsnum(len(r), 2)
		for i, o := range r {
			xnow := rowsnum(i+1, div)
			ysum := math.Min(len(r)-(xnow-1)*div, div)
			ynow := i%div + 1
			switch xnow {
			case 1:
				canvas.DrawString(string(o), -offest(xsum, xnow, tw)+115, offest(9, ynow, th)+320.0)
			case 2:
				canvas.DrawString(string(o), -offest(xsum, xnow, tw)+115, offest(9, ynow+(9-ysum), th)+320.0)
			}
		}
	}
	return writer.WriteTo(canvas.Image(), f)
}

func offest(total, now int, distance float64) float64 {
	if total%2 == 0 {
		return (float64(now-total/2) - 1) * distance
	}
	return (float64(now-total/2) - 1.5) * distance
}

func rowsnum(total, div int) int {
	temp := total / div
	if total%div != 0 {
		temp++
	}
	return temp
}

func getIP(r *http.Request) string {
	forwarded := r.Header.Get("X-FORWARDED-FOR")
	if forwarded != "" {
		return forwarded
	}
	return r.RemoteAddr
}

func methodis(m string, resp http.ResponseWriter, req *http.Request) bool {
	logrus.Debugf("[methodis] %v from %v.", req.Method, getIP(req))
	if req.Method != m {
		http.Error(resp, "405 Method Not Allowed", http.StatusMethodNotAllowed)
		return false
	}
	return true
}
