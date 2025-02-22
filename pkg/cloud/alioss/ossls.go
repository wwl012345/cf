package alioss

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	log "github.com/sirupsen/logrus"
	"githubu.com/teamssix/cf/pkg/cloud"
	"githubu.com/teamssix/cf/pkg/util"
	"githubu.com/teamssix/cf/pkg/util/cmdutil"
)

type Bucket = cloud.Resource

type Object struct {
	BucketName   string
	ObjectNumber int
	ObjectSize   int64
}

type Acl struct {
	BucketName string
	Acl        string
}

type error interface {
	Error() string
}

var (
	OSSCacheFilePath = cmdutil.ReturnOSSCacheFile()
	header           = []string{"序号 (SN)", "名称 (Name)", "存储桶 ACL (Bucket ACL)", "对象数量 (Object Number)", "存储桶大小 (Bucket Size)", "区域 (Region)", "存储桶地址 (Bucket URL)"}
)

func (o *OSSCollector) ListBuckets() ([]Bucket, error) {
	region := cloud.GetGlobalRegions()[0]
	o.OSSClient(region)
	var size = 10
	var out []Bucket
	marker := oss.Marker("")
	var err error
	for {
		var lbr oss.ListBucketsResult
		lbr, err = o.Client.ListBuckets(oss.MaxKeys(size), marker)
		marker = oss.Marker(lbr.NextMarker)
		for _, bucket := range lbr.Buckets {
			obj := Bucket{Name: bucket.Name,
				Region: bucket.Location[4:],
			}
			out = append(out, obj)
		}
		if !lbr.IsTruncated {
			break
		}
	}
	return out, err
}

func (o *OSSCollector) ListObjects() []Object {
	var size = 10
	var out []Object
	marker := oss.Marker("")

	OSSCollector := &OSSCollector{}
	Buckets, err := OSSCollector.ListBuckets()
	util.HandleErr(err)

	for _, j := range Buckets {
		BucketName := j.Name
		region := j.Region
		o.OSSClient(region)
		bucket, err := o.Client.Bucket(BucketName)
		util.HandleErr(err)

		lor, err := bucket.ListObjects(oss.MaxKeys(size), marker)
		util.HandleErr(err)
		marker = oss.Marker(lor.NextMarker)
		num := len(lor.Objects)
		var ObjectSize int64
		for _, k := range lor.Objects {
			ObjectSize = ObjectSize + k.Size
		}
		log.Debugf("在 %s 存储桶中找到了 %d 个对象 (Found %d Objects in %s Bucket)", BucketName, num, num, BucketName)
		obj := Object{
			BucketName:   BucketName,
			ObjectNumber: num,
			ObjectSize:   ObjectSize,
		}
		out = append(out, obj)
	}
	return out
}

func (o *OSSCollector) GetBucketACL() []Acl {
	OSSCollector := &OSSCollector{}
	Buckets, err := OSSCollector.ListBuckets()
	util.HandleErr(err)

	var out []Acl
	for _, j := range Buckets {
		BucketName := j.Name
		region := j.Region
		o.OSSClient(region)
		gbar, err := o.Client.GetBucketACL(BucketName)
		util.HandleErr(err)

		BucketACL := gbar.ACL
		if BucketACL == "private" {
			BucketACL = "私有 (Private)"
		} else if BucketACL == "public-read" {
			BucketACL = "公共读 (Public Read)"
		} else if BucketACL == "public-read-write" {
			BucketACL = "公共读写 (Public Read Write)"
		}

		obj := Acl{
			BucketName: BucketName,
			Acl:        BucketACL,
		}
		out = append(out, obj)
	}
	return out
}

func PrintBucketsListRealTime(region string) {
	OSSCollector := &OSSCollector{}

	Buckets, err := OSSCollector.ListBuckets()
	if err != nil {
		if strings.Contains(err.Error(), "You are forbidden to list buckets.") {
			log.Errorln("当前凭证不具备 OSS 的读取权限，无法获取 OSS 数据。 (OSS data is not available because the current credential does not have read access to OSS.)")
			os.Exit(0)
		} else {
			util.HandleErr(err)
		}
	}

	log.Debugf("获取到 %d 条 OSS Bucket 信息 (Obtained %d OSS Bucket information)", len(Buckets), len(Buckets))

	Objects := OSSCollector.ListObjects()
	ACL := OSSCollector.GetBucketACL()

	var num = 0
	for _, o := range Buckets {
		if region == "all" {
			num = len(Buckets)
		} else {
			if region == o.Region {
				num = num + 1
			}
		}
	}
	var data = make([][]string, num)
	num = 0
	for i, o := range Buckets {
		if region == "all" {
			SN := strconv.Itoa(i + 1)
			ObjectNumber := strconv.Itoa(Objects[i].ObjectNumber)
			ObjectSize := formatFileSize(Objects[i].ObjectSize)
			BucketACL := ACL[i].Acl
			BucketURL := fmt.Sprintf("https://%s.oss-%s.aliyuncs.com", o.Name, o.Region)
			data[i] = []string{SN, o.Name, BucketACL, ObjectNumber, ObjectSize, o.Region, BucketURL}
		} else {
			if region == o.Region {
				ObjectNumber := strconv.Itoa(Objects[i].ObjectNumber)
				ObjectSize := formatFileSize(Objects[i].ObjectSize)
				BucketACL := ACL[i].Acl
				BucketURL := fmt.Sprintf("https://%s.oss-%s.aliyuncs.com", o.Name, o.Region)
				data[num] = []string{strconv.Itoa(num + 1), o.Name, BucketACL, ObjectNumber, ObjectSize, o.Region, BucketURL}
				num = num + 1
			}
		}
	}
	var td = cloud.TableData{Header: header, Body: data}
	if len(data) == 0 {
		log.Info("没有存储桶 (No Bucket)")
		cmdutil.WriteCacheFile(td, OSSCacheFilePath)
	} else {
		Caption := "OSS 资源 (OSS resources)"
		cloud.PrintTable(td, Caption)
		cmdutil.WriteCacheFile(td, OSSCacheFilePath)
	}
}

func PrintBucketsListHistory(region string) {
	if cmdutil.FileExists(OSSCacheFilePath) {
		cmdutil.PrintCacheFile(OSSCacheFilePath, header, region, "all")
	} else {
		PrintBucketsListRealTime(region)
	}
}

func PrintBucketsList(region string, lsFlushCache bool) {
	if lsFlushCache {
		PrintBucketsListRealTime(region)
	} else {
		PrintBucketsListHistory(region)
	}
}
