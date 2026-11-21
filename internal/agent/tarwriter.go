// tar 打包：多根目录遍历 + 排除剪枝，tar 内条目名用去斜杠的绝对路径，
// 恢复时补回前导 / 即可还原（M3 恢复闭环依赖此约定）。
package agent

import (
	"archive/tar"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// newTarWriter 建立基于压缩流的 tar writer。
func newTarWriter(w io.Writer) *tar.Writer {
	return tar.NewWriter(w)
}

// writePathsToTar 把多个根目录按序写入 tar；excludes 为 doublestar 绝对路径 glob。
func writePathsToTar(tw *tar.Writer, paths, excludes []string) error {
	for _, root := range paths {
		if err := writeRootToTar(tw, root, excludes); err != nil {
			return err
		}
	}
	return nil
}

func writeRootToTar(tw *tar.Writer, root string, excludes []string) error {
	// 排除规则同样适用于根目录本身（例如临时把某根排除）
	if hit, err := matchedExcludes(root, excludes); err != nil {
		return err
	} else if hit {
		return nil
	}
	// WalkDir 对符号链接目录不跟随（默认 lstat 语义），链接按链接写入，避免循环与越界
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// 走不到的条目（权限等）跳过：备份语义是尽力而为，单条不可读不应让整体失败
			return nil
		}
		hit, err := matchedExcludes(p, excludes)
		if err != nil {
			return err
		}
		if hit {
			if d.IsDir() {
				return filepath.SkipDir // 目录命中排除规则：整棵剪枝
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		switch {
		case d.IsDir():
			return writeTarDir(tw, p, info)
		case info.Mode()&os.ModeSymlink != 0:
			return writeTarSymlink(tw, p, info)
		case info.Mode().IsRegular():
			return writeTarFile(tw, p, info)
		default:
			return nil // socket / device 等特殊文件不进备份
		}
	})
}

func writeTarDir(tw *tar.Writer, path string, info fs.FileInfo) error {
	hdr, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return fmt.Errorf("构造目录头失败 %s: %w", path, err)
	}
	hdr.Format = tar.FormatPAX
	hdr.Name = tarName(path) + "/" // 目录名以斜杠结尾是 tar 惯例
	hdr.Linkname = ""
	return tw.WriteHeader(hdr)
}

func writeTarFile(tw *tar.Writer, path string, info fs.FileInfo) error {
	f, err := os.Open(path)
	if err != nil {
		return nil // 打不开的文件跳过，不让整体失败
	}
	defer f.Close()
	hdr, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return fmt.Errorf("构造文件头失败 %s: %w", path, err)
	}
	hdr.Format = tar.FormatPAX
	hdr.Name = tarName(path)
	hdr.Linkname = ""
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("写文件头失败 %s: %w", path, err)
	}
	if _, err := io.Copy(tw, f); err != nil {
		return fmt.Errorf("写文件内容失败 %s: %w", path, err)
	}
	return nil
}

func writeTarSymlink(tw *tar.Writer, path string, info fs.FileInfo) error {
	link, err := os.Readlink(path)
	if err != nil {
		return nil // 读不出链接目标则跳过
	}
	hdr, err := tar.FileInfoHeader(info, link)
	if err != nil {
		return fmt.Errorf("构造链接头失败 %s: %w", path, err)
	}
	hdr.Format = tar.FormatPAX
	hdr.Name = tarName(path)
	return tw.WriteHeader(hdr)
}

// tarName 把绝对路径转成 tar 条目名：去前导斜杠，恢复时补 / 还原绝对路径。
func tarName(absPath string) string {
	return strings.TrimPrefix(absPath, "/")
}
