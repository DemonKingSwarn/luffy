// Copyright 2020 The Libc Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build freebsd && !386

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
	// trc("%T timep=%+v", time.Time_t(0), *(*time.Time_t)(unsafe.Pointer(timep)))
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
	localtime.Ftm_gmtoff = int64(off)
	localtime.Ftm_zone = 0
	// trc("%T localtime=%+v", localtime, localtime)
	return uintptr(unsafe.Pointer(&localtime))
}

// struct tm *localtime_r(const time_t *timep, struct tm *result);
func Xlocaltime_r(_ *TLS, timep, result uintptr) uintptr {
	// trc("%T timep=%+v", time.Time_t(0), *(*time.Time_t)(unsafe.Pointer(timep)))
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
	(*time.Tm)(unsafe.Pointer(result)).Ftm_gmtoff = int64(off)
	(*time.Tm)(unsafe.Pointer(result)).Ftm_zone = 0
	// trc("%T localtime_r=%+v", localtime, (*time.Tm)(unsafe.Pointer(result)))
	return result
}

func X__strchrnul(tls *TLS, s uintptr, c int32) uintptr { /* strchrnul.c:10:6: */
	if __ccgo_strace {
		trc("tls=%v s=%v c=%v, (%v:)", tls, s, c, origin(2))
	}
	c = int32(uint8(c))
	if !(c != 0) {
		return s + uintptr(Xstrlen(tls, s))
	}
	var w uintptr
	for ; uintptr_t(s)%uintptr_t(unsafe.Sizeof(size_t(0))) != 0; s++ {
		if !(int32(*(*int8)(unsafe.Pointer(s))) != 0) || int32(*(*uint8)(unsafe.Pointer(s))) == c {
			return s
		}
	}
	var k size_t = Uint64(Uint64FromInt32(-1)) / uint64(255) * size_t(c)
	for w = s; !((*(*uint64)(unsafe.Pointer(w))-Uint64(Uint64FromInt32(-1))/uint64(255)) & ^*(*uint64)(unsafe.Pointer(w)) & (Uint64(Uint64FromInt32(-1))/uint64(255)*uint64(255/2+1)) != 0) && !((*(*uint64)(unsafe.Pointer(w))^k-Uint64(Uint64FromInt32(-1))/uint64(255)) & ^(*(*uint64)(unsafe.Pointer(w))^k) & (Uint64(Uint64FromInt32(-1))/uint64(255)*uint64(255/2+1)) != 0); w += 8 {
	}
	s = w
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

	if !(var1 != 0) || !(int32(AssignUint64(&l1, size_t((int64(X__strchrnul(tls, var1, '='))-int64(var1))/1))) != 0) || *(*int8)(unsafe.Pointer(var1 + uintptr(l1))) != 0 {
		*(*int32)(unsafe.Pointer(X___errno_location(tls))) = 22
		return -1
	}
	if !(overwrite != 0) && Xgetenv(tls, var1) != 0 {
		return 0
	}

	l2 = Xstrlen(tls, value)
	s = Xmalloc(tls, l1+l2+uint64(2))
	if !(s != 0) {
		return -1
	}
	Xmemcpy(tls, s, var1, l1)
	*(*int8)(unsafe.Pointer(s + uintptr(l1))) = int8('=')
	Xmemcpy(tls, s+uintptr(l1)+uintptr(1), value, l2+uint64(1))
	return X__putenv(tls, s, l1, s)
}

func X__putenv(tls *TLS, s uintptr, l size_t, r uintptr) int32 { /* putenv.c:8:5: */
	if __ccgo_strace {
		trc("tls=%v s=%v l=%v r=%v, (%v:)", tls, s, l, r, origin(2))
	}
	var i size_t
	var newenv uintptr
	var tmp uintptr
	//TODO for (char **e = __environ; *e; e++, i++)
	var e uintptr
	i = uint64(0)
	if !(Environ() != 0) {
		goto __1
	}
	//TODO for (char **e = __environ; *e; e++, i++)
	e = Environ()
__2:
	if !(*(*uintptr)(unsafe.Pointer(e)) != 0) {
		goto __4
	}
	if !!(Xstrncmp(tls, s, *(*uintptr)(unsafe.Pointer(e)), l+uint64(1)) != 0) {
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
	e += 8
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
	newenv = Xrealloc(tls, _soldenv, uint64(unsafe.Sizeof(uintptr(0)))*(i+uint64(2)))
	if !!(newenv != 0) {
		goto __8
	}
	goto oom
__8:
	;
	goto __7
__6:
	newenv = Xmalloc(tls, uint64(unsafe.Sizeof(uintptr(0)))*(i+uint64(2)))
	if !!(newenv != 0) {
		goto __9
	}
	goto oom
__9:
	;
	if !(i != 0) {
		goto __10
	}
	Xmemcpy(tls, newenv, Environ(), uint64(unsafe.Sizeof(uintptr(0)))*i)
__10:
	;
	Xfree(tls, _soldenv)
__7:
	;
	*(*uintptr)(unsafe.Pointer(newenv + uintptr(i)*8)) = s
	*(*uintptr)(unsafe.Pointer(newenv + uintptr(i+uint64(1))*8)) = uintptr(0)
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
	//TODO for (size_t i=0; i < env_alloced_n; i++)
	var i size_t = uint64(0)
	for ; i < _senv_alloced_n; i++ {
		if *(*uintptr)(unsafe.Pointer(_senv_alloced + uintptr(i)*8)) == old {
			*(*uintptr)(unsafe.Pointer(_senv_alloced + uintptr(i)*8)) = new
			Xfree(tls, old)
			return
		} else if !(int32(*(*uintptr)(unsafe.Pointer(_senv_alloced + uintptr(i)*8))) != 0) && new != 0 {
			*(*uintptr)(unsafe.Pointer(_senv_alloced + uintptr(i)*8)) = new
			new = uintptr(0)
		}
	}
	if !(new != 0) {
		return
	}
	var t uintptr = Xrealloc(tls, _senv_alloced, uint64(unsafe.Sizeof(uintptr(0)))*(_senv_alloced_n+uint64(1)))
	if !(t != 0) {
		return
	}
	*(*uintptr)(unsafe.Pointer(AssignPtrUintptr(uintptr(unsafe.Pointer(&_senv_alloced)), t) + uintptr(PostIncUint64(&_senv_alloced_n, 1))*8)) = new
}
