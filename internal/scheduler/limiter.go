package scheduler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
)

var (
	ErrLeaseReleased = errors.New("lease 已释放")
)

type Caps struct {
	// GlobalMax 是全局并行上限（对应 docs/02 的 GLOBAL_MAX）。
	// 为避免“未初始化导致无限并行”，Caps 必须显式设置。
	GlobalMax int

	// PerTeam 是每个 Team 的并行上限（对应 TEAM_MAX）。缺省时回退到 GlobalMax。
	PerTeam map[string]int

	// PerRole 是每个 role/agent 的并行上限（对应 AGENT_MAX）。缺省时回退到 GlobalMax。
	PerRole map[string]int
}

func (c Caps) Validate() error {
	if c.GlobalMax <= 0 {
		return fmt.Errorf("GlobalMax 必须 >0")
	}
	for teamID, cap := range c.PerTeam {
		if strings.TrimSpace(teamID) == "" {
			return fmt.Errorf("PerTeam 存在空 team_id")
		}
		if cap <= 0 {
			return fmt.Errorf("PerTeam[%s] 必须 >0", teamID)
		}
	}
	for role, cap := range c.PerRole {
		if strings.TrimSpace(role) == "" {
			return fmt.Errorf("PerRole 存在空 role")
		}
		if cap <= 0 {
			return fmt.Errorf("PerRole[%s] 必须 >0", role)
		}
	}
	return nil
}

type Key struct {
	TeamID string
	Role   string
}

func (k Key) Validate() error {
	if strings.TrimSpace(k.TeamID) == "" && strings.TrimSpace(k.Role) == "" {
		return fmt.Errorf("Key 至少要提供 team_id 或 role")
	}
	return nil
}

type Snapshot struct {
	Caps        Caps
	InUseGlobal int
	InUseTeam   map[string]int
	InUseRole   map[string]int
}

type Limiter struct {
	mu sync.Mutex

	caps Caps

	inUseGlobal int
	inUseTeam   map[string]int
	inUseRole   map[string]int

	notify chan struct{}
}

func NewLimiter(caps Caps) (*Limiter, error) {
	if err := caps.Validate(); err != nil {
		return nil, err
	}
	return &Limiter{
		caps:      cloneCaps(caps),
		inUseTeam: map[string]int{},
		inUseRole: map[string]int{},
		notify:    make(chan struct{}),
	}, nil
}

func (l *Limiter) UpdateCaps(caps Caps) error {
	if err := caps.Validate(); err != nil {
		return err
	}
	l.mu.Lock()
	l.caps = cloneCaps(caps)
	l.broadcastLocked()
	l.mu.Unlock()
	return nil
}

func (l *Limiter) Snapshot() Snapshot {
	l.mu.Lock()
	defer l.mu.Unlock()

	inUseTeam := make(map[string]int, len(l.inUseTeam))
	for k, v := range l.inUseTeam {
		inUseTeam[k] = v
	}
	inUseRole := make(map[string]int, len(l.inUseRole))
	for k, v := range l.inUseRole {
		inUseRole[k] = v
	}

	return Snapshot{
		Caps:        cloneCaps(l.caps),
		InUseGlobal: l.inUseGlobal,
		InUseTeam:   inUseTeam,
		InUseRole:   inUseRole,
	}
}

func (l *Limiter) TryAcquire(key Key) (Lease, bool, error) {
	if err := key.Validate(); err != nil {
		return Lease{}, false, err
	}
	l.mu.Lock()
	lease, ok := l.tryAcquireLocked(key)
	l.mu.Unlock()
	return lease, ok, nil
}

func (l *Limiter) Acquire(ctx context.Context, key Key) (Lease, error) {
	if err := key.Validate(); err != nil {
		return Lease{}, err
	}
	for {
		l.mu.Lock()
		lease, ok := l.tryAcquireLocked(key)
		if ok {
			l.mu.Unlock()
			return lease, nil
		}
		ch := l.notify
		l.mu.Unlock()

		select {
		case <-ctx.Done():
			return Lease{}, ctx.Err()
		case <-ch:
		}
	}
}

func (l *Limiter) tryAcquireLocked(key Key) (Lease, bool) {
	if !l.canAcquireLocked(key) {
		return Lease{}, false
	}
	l.inUseGlobal++
	if strings.TrimSpace(key.TeamID) != "" {
		l.inUseTeam[key.TeamID]++
	}
	if strings.TrimSpace(key.Role) != "" {
		l.inUseRole[key.Role]++
	}
	return newLease(l, key), true
}

func (l *Limiter) canAcquireLocked(key Key) bool {
	if l.inUseGlobal >= l.caps.GlobalMax {
		return false
	}
	if strings.TrimSpace(key.TeamID) != "" {
		cap := l.teamCapLocked(key.TeamID)
		if l.inUseTeam[key.TeamID] >= cap {
			return false
		}
	}
	if strings.TrimSpace(key.Role) != "" {
		cap := l.roleCapLocked(key.Role)
		if l.inUseRole[key.Role] >= cap {
			return false
		}
	}
	return true
}

func (l *Limiter) teamCapLocked(teamID string) int {
	if l.caps.PerTeam != nil {
		if cap, ok := l.caps.PerTeam[teamID]; ok {
			return cap
		}
	}
	return l.caps.GlobalMax
}

func (l *Limiter) roleCapLocked(role string) int {
	if l.caps.PerRole != nil {
		if cap, ok := l.caps.PerRole[role]; ok {
			return cap
		}
	}
	return l.caps.GlobalMax
}

func (l *Limiter) release(key Key) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.inUseGlobal <= 0 {
		return fmt.Errorf("release underflow: global")
	}
	l.inUseGlobal--

	if strings.TrimSpace(key.TeamID) != "" {
		n := l.inUseTeam[key.TeamID] - 1
		if n < 0 {
			return fmt.Errorf("release underflow: team=%s", key.TeamID)
		}
		if n == 0 {
			delete(l.inUseTeam, key.TeamID)
		} else {
			l.inUseTeam[key.TeamID] = n
		}
	}

	if strings.TrimSpace(key.Role) != "" {
		n := l.inUseRole[key.Role] - 1
		if n < 0 {
			return fmt.Errorf("release underflow: role=%s", key.Role)
		}
		if n == 0 {
			delete(l.inUseRole, key.Role)
		} else {
			l.inUseRole[key.Role] = n
		}
	}

	l.broadcastLocked()
	return nil
}

func (l *Limiter) broadcastLocked() {
	close(l.notify)
	l.notify = make(chan struct{})
}

func cloneCaps(c Caps) Caps {
	out := Caps{
		GlobalMax: c.GlobalMax,
	}
	if c.PerTeam != nil {
		out.PerTeam = make(map[string]int, len(c.PerTeam))
		for k, v := range c.PerTeam {
			out.PerTeam[k] = v
		}
	}
	if c.PerRole != nil {
		out.PerRole = make(map[string]int, len(c.PerRole))
		for k, v := range c.PerRole {
			out.PerRole[k] = v
		}
	}
	return out
}

type leaseState struct {
	released atomic.Bool
}

type Lease struct {
	limiter *Limiter
	key     Key
	state   *leaseState
}

func newLease(l *Limiter, key Key) Lease {
	return Lease{
		limiter: l,
		key:     key,
		state:   &leaseState{},
	}
}

func (l Lease) Release() error {
	if l.limiter == nil || l.state == nil {
		return fmt.Errorf("lease 无效")
	}
	if l.state.released.Swap(true) {
		return ErrLeaseReleased
	}
	return l.limiter.release(l.key)
}
