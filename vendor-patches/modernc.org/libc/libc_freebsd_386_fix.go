// Copyright 2020 The Libc Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build freebsd && 386

package libc // import "modernc.org/libc"

import (
	gotime "time"
	"unsafe"

	"golang.org/x/sys/unix"
	"modernc.org/libc/time"
)

var localtime time.Tm

// struct tm *localtime(const time_t *timep);
func Xlocaltime(_ *TLS, timep uintptr) uintptr {
	loc := getLocalLocation()
	ut := *(*time.Time_t)(unsafe.Pointer(timep))
	t := gotime.Unix(int64(ut), 0).In(loc)
	localtime.Ftm_sec = int32(t.Second())
	localtime.Ftm_min = int32(t.Minute())
	localtime.Ftm_hour = int32(t.Hour())
	localtime.Ftm_mday = int32(t.Day())
	localtime.Ftm_mon = int32(t.Month() - 1)
	localtime.Ftm_year = int32(t.Year() - 1900)
	localtime.Ftm_wday = int32(t.Weekday())
	localtime.Ftm_yday = int32(t.YearDay())
	localtime.Ftm_isdst = Bool32(isTimeDST(t))
	_, off := t.Zone()
	localtime.Ftm_gmtoff = int32(off) // int32 on freebsd/386
	localtime.Ftm_zone = 0
	return uintptr(unsafe.Pointer(&localtime))
}

// struct tm *localtime_r(const time_t *timep, struct tm *result);
func Xlocaltime_r(_ *TLS, timep, result uintptr) uintptr {
	loc := getLocalLocation()
	ut := *(*unix.Time_t)(unsafe.Pointer(timep))
	t := gotime.Unix(int64(ut), 0).In(loc)
	(*time.Tm)(unsafe.Pointer(result)).Ftm_sec = int32(t.Second())
	(*time.Tm)(unsafe.Pointer(result)).Ftm_min = int32(t.Minute())
	(*time.Tm)(unsafe.Pointer(result)).Ftm_hour = int32(t.Hour())
	(*time.Tm)(unsafe.Pointer(result)).Ftm_mday = int32(t.Day())
	(*time.Tm)(unsafe.Pointer(result)).Ftm_mon = int32(t.Month() - 1)
	(*time.Tm)(unsafe.Pointer(result)).Ftm_year = int32(t.Year() - 1900)
	(*time.Tm)(unsafe.Pointer(result)).Ftm_wday = int32(t.Weekday())
	(*time.Tm)(unsafe.Pointer(result)).Ftm_yday = int32(t.YearDay())
	(*time.Tm)(unsafe.Pointer(result)).Ftm_isdst = Bool32(isTimeDST(t))
	_, off := t.Zone()
	(*time.Tm)(unsafe.Pointer(result)).Ftm_gmtoff = int32(off) // int32 on freebsd/386
	(*time.Tm)(unsafe.Pointer(result)).Ftm_zone = 0
	return result
}

// X__strchrnul: simple byte-by-byte implementation for 32-bit.
// The 64-bit word-at-a-time optimization in libc_freebsd.go is not valid on
// freebsd/386 because size_t is uint32 there, not uint64.
func X__strchrnul(tls *TLS, s uintptr, c int32) uintptr { /* strchrnul.c:10:6: */
	if __ccgo_strace {
		trc("tls=%v s=%v c=%v, (%v:)", tls, s, c, origin(2))
	}
	c = int32(uint8(c))
	if !(c != 0) {
		return s + uintptr(Xstrlen(tls, s))
	}
	for ; *(*int8)(unsafe.Pointer(s)) != 0 && int32(*(*uint8)(unsafe.Pointer(s))) != c; s++ {
	}
	return s
}

var _soldenv uintptr /* putenv.c:22:14: */

// int setenv(const char *name, const char *value, int overwrite);
func Xsetenv(tls *TLS, var1 uintptr, value uintptr, overwrite int32) int32 { /* setenv.c:26:5: */
	if __ccgo_strace {
		trc("tls=%v var1=%v value=%v overwrite=%v, (%v:)", tls, var1, value, overwrite, origin(2))
	}
	var s uintptr
	var l1 size_t
	var l2 size_t

	if !(var1 != 0) || !(int32(AssignUint32(&l1, size_t((int64(X__strchrnul(tls, var1, '='))-int64(var1))/1))) != 0) || *(*int8)(unsafe.Pointer(var1 + uintptr(l1))) != 0 {
		*(*int32)(unsafe.Pointer(X___errno_location(tls))) = 22
		return -1
	}
	if !(overwrite != 0) && Xgetenv(tls, var1) != 0 {
		return 0
	}

	l2 = Xstrlen(tls, value)
	s = Xmalloc(tls, l1+l2+size_t(2))
	if !(s != 0) {
		return -1
	}
	Xmemcpy(tls, s, var1, l1)
	*(*int8)(unsafe.Pointer(s + uintptr(l1))) = int8('=')
	Xmemcpy(tls, s+uintptr(l1)+uintptr(1), value, l2+size_t(1))
	return X__putenv(tls, s, l1, s)
}

func X__putenv(tls *TLS, s uintptr, l size_t, r uintptr) int32 { /* putenv.c:8:5: */
	if __ccgo_strace {
		trc("tls=%v s=%v l=%v r=%v, (%v:)", tls, s, l, r, origin(2))
	}
	var i size_t
	var newenv uintptr
	var tmp uintptr
	var e uintptr
	i = size_t(0)
	if !(Environ() != 0) {
		goto __1
	}
	e = Environ()
__2:
	if !(*(*uintptr)(unsafe.Pointer(e)) != 0) {
		goto __4
	}
	if !!(Xstrncmp(tls, s, *(*uintptr)(unsafe.Pointer(e)), l+size_t(1)) != 0) {
		goto __5
	}
	tmp = *(*uintptr)(unsafe.Pointer(e))
	*(*uintptr)(unsafe.Pointer(e)) = s
	X__env_rm_add(tls, tmp, r)
	return 0
__5:
	;
	goto __3
__3:
	e += 4 // pointer size is 4 on 386
	i++
	goto __2
	goto __4
__4:
	;
__1:
	;
	if !(Environ() == _soldenv) {
		goto __6
	}
	newenv = Xrealloc(tls, _soldenv, size_t(unsafe.Sizeof(uintptr(0)))*(i+size_t(2)))
	if !!(newenv != 0) {
		goto __8
	}
	goto oom
__8:
	;
	goto __7
__6:
	newenv = Xmalloc(tls, size_t(unsafe.Sizeof(uintptr(0)))*(i+size_t(2)))
	if !!(newenv != 0) {
		goto __9
	}
	goto oom
__9:
	;
	if !(i != 0) {
		goto __10
	}
	Xmemcpy(tls, newenv, Environ(), size_t(unsafe.Sizeof(uintptr(0)))*i)
__10:
	;
	Xfree(tls, _soldenv)
__7:
	;
	*(*uintptr)(unsafe.Pointer(newenv + uintptr(i)*4)) = s
	*(*uintptr)(unsafe.Pointer(newenv + uintptr(i+size_t(1))*4)) = uintptr(0)
	*(*uintptr)(unsafe.Pointer(EnvironP())) = AssignPtrUintptr(uintptr(unsafe.Pointer(&_soldenv)), newenv)
	if !(r != 0) {
		goto __11
	}
	X__env_rm_add(tls, uintptr(0), r)
__11:
	;
	return 0
oom:
	Xfree(tls, r)
	return -1
}

var _senv_alloced uintptr  /* setenv.c:7:14: */
var _senv_alloced_n size_t /* setenv.c:8:16: */

func X__env_rm_add(tls *TLS, old uintptr, new uintptr) { /* setenv.c:5:6: */
	if __ccgo_strace {
		trc("tls=%v old=%v new=%v, (%v:)", tls, old, new, origin(2))
	}
	var i size_t = size_t(0)
	for ; i < _senv_alloced_n; i++ {
		if *(*uintptr)(unsafe.Pointer(_senv_alloced + uintptr(i)*4)) == old {
			*(*uintptr)(unsafe.Pointer(_senv_alloced + uintptr(i)*4)) = new
			Xfree(tls, old)
			return
		} else if !(int32(*(*uintptr)(unsafe.Pointer(_senv_alloced + uintptr(i)*4))) != 0) && new != 0 {
			*(*uintptr)(unsafe.Pointer(_senv_alloced + uintptr(i)*4)) = new
			new = uintptr(0)
		}
	}
	if !(new != 0) {
		return
	}
	var t uintptr = Xrealloc(tls, _senv_alloced, size_t(unsafe.Sizeof(uintptr(0)))*(_senv_alloced_n+size_t(1)))
	if !(t != 0) {
		return
	}
	*(*uintptr)(unsafe.Pointer(AssignPtrUintptr(uintptr(unsafe.Pointer(&_senv_alloced)), t) + uintptr(PostIncUint32(&_senv_alloced_n, 1))*4)) = new
}
