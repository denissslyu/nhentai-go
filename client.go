package nhentai

import (
	"errors"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/json-iterator/go"
	"github.com/json-iterator/go/extra"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

var json jsoniter.API

func init() {
	extra.RegisterFuzzyDecoders()
	json = jsoniter.ConfigCompatibleWithStandardLibrary
}

const MirrorOrigin = "nhentai.net"

// Client nHentai客户端
type Client struct {
	// http.Client 继承HTTP客户端
	http.Client
	cookie    string
	userAgent string
	proxy     string
}

func NewClient() *Client {
	return &Client{}
}

func (c *Client) SetCookie(cookie string) *Client {
	c.cookie = cookie
	return c
}

func (c *Client) SetUserAgent(userAgent string) *Client {
	c.userAgent = userAgent
	return c
}

func (c *Client) SetProxy(proxyURL string) error {
	if proxyURL == "" {
		c.Transport = &http.Transport{
			Proxy: nil,
		}
		return nil
	}
	// 解析代理地址
	proxy, err := url.Parse(proxyURL)
	if err != nil {
		return err
	}

	c.Transport = &http.Transport{
		Proxy: http.ProxyURL(proxy),
	}

	return nil
}

// Comics 列出漫画
// https://nhentai.net/?page=1
func (c *Client) Comics(page int) (*ComicPageData, error) {
	urlStr := fmt.Sprintf("https://%s/?page=%d", MirrorOrigin, page)
	return c.parsePage(urlStr)
}

func (c *Client) ComicsByCondition(conditions []Condition, page int) (*ComicPageData, error) {
	builder := strings.Builder{}
	for i := range conditions {
		conditions[i].Type = strings.TrimSpace(conditions[i].Type)
		if len(conditions[i].Type) == 0 {
			return nil, errors.New("BLANK TYPE")
		} else if "string" == conditions[i].Type {
			if conditions[i].Exclude {
				builder.WriteString("-")
			}
			builder.WriteString("\"")
			builder.WriteString(strings.ReplaceAll(strings.TrimSpace(conditions[i].Content), "\"", ""))
			builder.WriteString("\"")
			builder.WriteString(" ")
		}
	}
	return c.ComicByRawCondition(builder.String(), page)
}

// ComicByRawCondition 搜索
// https://nhentai.net/search/?q=${urlEncode(conditions)}&page=${page}
func (c *Client) ComicByRawCondition(conditions string, page int) (*ComicPageData, error) {
	conditions = strings.TrimSpace(conditions)
	if len(conditions) == 0 {
		return c.Comics(page)
	}
	urlStr := fmt.Sprintf("https://%s/search/?q=%s&page=%d", MirrorOrigin, url.QueryEscape(conditions), page)
	return c.parsePage(urlStr)
}

// ComicsByTagName 列出标签下的漫画
// https://nhentai.net/tag/group/?page=1
func (c *Client) ComicsByTagName(tag string, page int) (*ComicPageData, error) {
	urlStr := fmt.Sprintf("https://%s/tag/%s/?page=%d", MirrorOrigin, tag, page)
	return c.parsePage(urlStr)
}

// parsePage 获取页面上的漫画列表
func (c *Client) parsePage(urlStr string) (*ComicPageData, error) {
	doc, err := c.parseUrlToDoc(urlStr)
	if err != nil {
		return nil, err
	}
	if strings.Contains(doc.Text(), "No results found") {
		return &ComicPageData{
			PageData: PageData{
				PageCount: 0,
			},
			Records: make([]ComicSimple, 0),
		}, nil
	}
	var divSelection *goquery.Selection
	doc.Find(".container.index-container:not(.index-popular)").Each(func(i int, selection *goquery.Selection) {
		divSelection = selection
	})
	if divSelection == nil {
		return nil, errors.New("NOT MATCH CONTAINER")
	}
	gallerySelection := divSelection.Find("div.gallery")
	galleries := make([]ComicSimple, gallerySelection.Size())
	gallerySelection.Each(func(i int, selection *goquery.Selection) {
		idStr, _ := selection.Find("a").First().Attr("href")
		idStr = strings.TrimPrefix(idStr, "/g/")
		idStr = strings.TrimSuffix(idStr, "/")
		id, _ := strconv.Atoi(idStr)
		title := selection.Find(".caption").Text()
		thumb, thumbWidth, thumbHeight, mediaId := c.parseCover(selection)
		tagIdsStr, _ := selection.Attr("data-tags")
		tsp := strings.Split(tagIdsStr, " ")
		tagIds := make([]int, len(tsp))
		for i2 := range tsp {
			tagIds[i2], _ = strconv.Atoi(tsp[i2])
		}
		lang := lang(tagIds)
		galleries[i] = ComicSimple{
			Id:          id,
			Title:       title,
			MediaId:     mediaId,
			TagIds:      tagIds,
			Lang:        lang,
			Thumb:       thumb,
			ThumbWidth:  thumbWidth,
			ThumbHeight: thumbHeight,
		}
	})
	lastPage, err := c.parseLastPage(doc)
	if err != nil {
		return nil, err
	}
	return &ComicPageData{
		PageData: PageData{
			PageCount: lastPage,
		},
		Records: galleries,
	}, nil
}

// parseCover 分析媒体信息
func (c *Client) parseCover(selection *goquery.Selection) (string, int, int, int) {
	lazyload := selection.Find(".lazyload")
	thumb, _ := lazyload.Attr("data-src")
	thumbWidthStr, _ := lazyload.Attr("width")
	thumbHeightStr, _ := lazyload.Attr("height")
	width, _ := strconv.Atoi(thumbWidthStr)
	thumbHeight, _ := strconv.Atoi(thumbHeightStr)
	mediaIdStr := thumb[strings.Index(thumb, "galleries")+10 : strings.LastIndex(thumb, "/")]
	mediaId, _ := strconv.Atoi(mediaIdStr)
	return thumb, width, thumbHeight, mediaId
}

func (c *Client) Get(url string) (*http.Response, error) {
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Add("Cookie", c.cookie)
	request.Header.Add("User-Agent", c.userAgent)
	return c.Client.Do(request)
}

// ComicInfo 获取漫画的信息
func (c *Client) ComicInfo(id int) (*ComicInfo, error) {
	urlStr := fmt.Sprintf("https://%s/api/gallery/%d", MirrorOrigin, id)
	rsp, err := c.Get(urlStr)
	if err != nil {
		return nil, err
	}
	defer rsp.Body.Close()
	buff, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		return nil, err
	}
	var comicInfo ComicInfo
	err = json.Unmarshal(buff, &comicInfo)
	if err != nil {
		return nil, err
	}
	// []Tag
	return &comicInfo, nil
}

// Tags 获取标签
// https://nhentai.net/tags/?page=1
func (c *Client) Tags(page int) (*TagPageData, error) {
	urlStr := fmt.Sprintf("https://%s/tags/?page=%d", MirrorOrigin, page)
	doc, err := c.parseUrlToDoc(urlStr)
	if err != nil {
		return nil, err
	}
	tags := c.parseTags(doc.Find("div.container#tag-container>section>a"))
	lastPage, err := c.parseLastPage(doc)
	if err != nil {
		return nil, err
	}
	return &TagPageData{
		PageData: PageData{
			PageCount: lastPage,
		},
		Records: tags,
	}, nil
}

// parseTags 解析标签数据
func (c *Client) parseTags(tagSelections *goquery.Selection) []TagPageTag {
	tags := make([]TagPageTag, tagSelections.Size())
	tagSelections.Each(func(i int, selection *goquery.Selection) {
		aClass, _ := selection.Attr("class")
		aClass = strings.TrimPrefix(aClass, "tag tag-")
		aClass = strings.TrimSpace(aClass)
		id, _ := strconv.Atoi(aClass)
		name := selection.Find(".name").Text()
		count := selection.Find(".count").Text()
		tags[i] = TagPageTag{
			Id:    id,
			Name:  name,
			Count: count,
		}
	})
	return tags
}

// parseLastPage 获取一共多少页
func (c *Client) parseLastPage(doc *goquery.Document) (int, error) {
	lastPageSelection := doc.Find(".pagination>.last")
	if lastPageSelection.Size() == 0 {
		// 最后一页
		return 0, nil
	}
	lastPageHref, ex := lastPageSelection.Attr("href")
	if !ex {
		return 0, errors.New("NOT MATCH PAGE")
	}
	pIndex := strings.Index(lastPageHref, "page=")
	if pIndex < 0 {
		return 0, errors.New("NOT MATCH PAGE")
	}
	lastPageHref = lastPageHref[pIndex+5:]
	lastPage, _ := strconv.Atoi(lastPageHref)
	return lastPage, nil
}

// parseUrlToDoc 从网址读取网页并且转换成document
func (c *Client) parseUrlToDoc(str string) (*goquery.Document, error) {
	rsp, err := c.Get(str)
	if err != nil {
		return nil, err
	}
	defer rsp.Body.Close()
	return goquery.NewDocumentFromReader(rsp.Body)
}

// CoverUrl 拼接封面的URL
// "https://t.nhentai.net/galleries/{media_id}/cover.{cover_ext}"
func (c *Client) CoverUrl(mediaId int, t string) string {
	return fmt.Sprintf("https://%s.%s/galleries/%d/cover.%s", getRandomSubDomainT(), MirrorOrigin, mediaId, c.GetExtension(t))
}

// ThumbnailUrl 拼接缩略图的URL
// "https://t2.nhentai.net/galleries/{media_id}/thumb.{thumbnail_ext}"
func (c *Client) ThumbnailUrl(mediaId int, t string) string {
	return fmt.Sprintf("https://%s.%s/galleries/%d/thumb.%s", getRandomSubDomainT(), MirrorOrigin, mediaId, c.GetExtension(t))
}

// PageUrl
// https://i.nhentai.net/galleries/{media_id}/{num}.{extension}
// {num} is {index + 1} (begin is 1)
func (c *Client) PageUrl(mediaId int, num int, t string) string {
	return fmt.Sprintf("https://%s.%s/galleries/%d/%d.%s", getRandomSubDomainI(), MirrorOrigin, mediaId, num, c.GetExtension(t))
}

// PageThumbnailUrl
// https://t5.nhentai.net/galleries/{media_id}/{num}t.{extension}
// {num} is {index + 1} (begin is 1)
func (c *Client) PageThumbnailUrl(mediaId int, num int, t string) string {
	return fmt.Sprintf("https://%s.%s/galleries/%d/%dt.%s", getRandomSubDomainT(), MirrorOrigin, mediaId, num, c.GetExtension(t))
}

// GetExtension 使用type获得拓展名
func (c *Client) GetExtension(t string) string {
	// Official only j
	if t == "j" {
		return "jpg"
	}
	// redundancy
	if t == "p" {
		return "png"
	}
	return ""
}
