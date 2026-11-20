package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/matchaction"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcmindset"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/authentication"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/input"
)

type NPCMindsetHandler struct {
	eventStore domain.EventStore[npcmindset.ID, npcmindset.Event, npcmindset.State]
	identifier domain.Identifier[npcmindset.ID]
	updateName npcmindset.UpdateNameFunc
	now        func() time.Time
}

func NewNPCMindsetHandler(
	eventStore domain.EventStore[npcmindset.ID, npcmindset.Event, npcmindset.State],
	identifier domain.Identifier[npcmindset.ID],
	updateName npcmindset.UpdateNameFunc,
	now func() time.Time,
) *NPCMindsetHandler {
	return &NPCMindsetHandler{
		eventStore: eventStore, identifier: identifier, updateName: updateName, now: now,
	}
}

// 変える操作の結果は、公開 ID と新しい revision だけにする（理由は ChangedPresetDeck と同じ）
type ChangedNPCMindset struct {
	ID       uuid.UUID
	Revision int64
}

// 準備中の 準備中の NPC の思考パターンを作る。登録のときの重みはすべて 1（domain の既定値）。操作した人は context のオペレーター。
// 入力の誤りは input.Errors、使われている名前（状態によらず、すべての NPC の思考パターンの中で）なら ErrNPCMindsetNameAlreadyInUse
func (c *NPCMindsetHandler) Register(ctx context.Context, name string) (ChangedNPCMindset, error) {
	registeredBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedNPCMindset{}, ErrNotOperator
	}
	npcMindsetName, invalid := newNPCMindsetName("name", name)
	if invalid != nil {
		return ChangedNPCMindset{}, input.Errors{invalid}
	}
	registered, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedNPCMindset, error) {
		id, publicID, err := c.identifier.Issue(ctx)
		if err != nil {
			return ChangedNPCMindset{}, err
		}
		if err := c.updateName(ctx, id, npcMindsetName); err != nil {
			return ChangedNPCMindset{}, err
		}
		registered, err := c.decideAndSave(ctx, npcmindset.InitialState(), &npcmindset.RegisterCommand{ID: id, PublicID: publicID, Name: npcMindsetName, RegisteredBy: registeredBy})
		if err != nil {
			return ChangedNPCMindset{}, err
		}
		return changedNPCMindset(registered), nil
	})
	if errors.Is(err, npcmindset.ErrNameAlreadyInUse) {
		return ChangedNPCMindset{}, ErrNPCMindsetNameAlreadyInUse
	}
	return registered, err
}

// revision は画面で見た NPC の思考パターンの版。無い・削除したなら ErrNPCMindsetNotFound、画面を開いた後にほかの操作で変わっていれば
// ErrRevisionConflict、使われている名前なら ErrNPCMindsetNameAlreadyInUse、準備中でなければ ErrNPCMindsetNotDraft
func (c *NPCMindsetHandler) Rename(ctx context.Context, id uuid.UUID, revision int64, name string) (ChangedNPCMindset, error) {
	renamedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedNPCMindset{}, ErrNotOperator
	}
	npcMindsetName, invalid := newNPCMindsetName("name", name)
	if invalid != nil {
		return ChangedNPCMindset{}, input.Errors{invalid}
	}
	renamed, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedNPCMindset, error) {
		current, err := c.load(ctx, id, revision)
		if err != nil {
			return ChangedNPCMindset{}, err
		}
		if current.Status != npcmindset.StatusDraft {
			return ChangedNPCMindset{}, npcmindset.ErrNotDraft
		}
		if err := c.updateName(ctx, current.ID, npcMindsetName); err != nil {
			return ChangedNPCMindset{}, err
		}
		renamed, err := c.decideAndSave(ctx, current, &npcmindset.RenameCommand{Name: npcMindsetName, RenamedBy: renamedBy})
		if err != nil {
			return ChangedNPCMindset{}, err
		}
		return changedNPCMindset(renamed), nil
	})
	if errors.Is(err, npcmindset.ErrNameAlreadyInUse) {
		return ChangedNPCMindset{}, ErrNPCMindsetNameAlreadyInUse
	}
	return renamed, mapNPCMindsetError(err)
}

// 行動の種類ごとの重みを、渡した全種類の重みに置き換える。入力の誤りはまとめて input.Errors で返す。項目名は weights.<番号>（同じ種類の
// 2 つ目以降は input.Duplicated、範囲外の重みは weights.<番号>.weight の input.OutOfRange。範囲は 0〜100）。
// 種類のすべてがそろっていなければ weights の input.Required（種類が多すぎる・知らない種類は、入力の型（enum）が作らせない）。
// 準備中でなければ ErrNPCMindsetNotDraft。同じ重みなら（変わらないので）何も変えずに成功する。ほかのエラーは Rename と同じ
func (c *NPCMindsetHandler) SetActionWeights(ctx context.Context, id uuid.UUID, revision int64, weights []ActionWeightInput) (ChangedNPCMindset, error) {
	changedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedNPCMindset{}, ErrNotOperator
	}
	actionWeights, err := parseActionWeights(weights)
	if err != nil {
		return ChangedNPCMindset{}, err
	}
	changed, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedNPCMindset, error) {
		current, err := c.load(ctx, id, revision)
		if err != nil {
			return ChangedNPCMindset{}, err
		}
		changed, err := c.decideAndSave(ctx, current, &npcmindset.SetActionWeightsCommand{Weights: actionWeights, ChangedBy: changedBy})
		if err != nil {
			return ChangedNPCMindset{}, err
		}
		return changedNPCMindset(changed), nil
	})
	return changed, mapNPCMindsetError(err)
}

// 採点基準ごとの重みを、渡した全基準の重みに置き換える。誤りの項目名と意味は SetActionWeights と同じ
func (c *NPCMindsetHandler) SetCriterionWeights(ctx context.Context, id uuid.UUID, revision int64, weights []CriterionWeightInput) (ChangedNPCMindset, error) {
	changedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedNPCMindset{}, ErrNotOperator
	}
	criterionWeights, err := parseCriterionWeights(weights)
	if err != nil {
		return ChangedNPCMindset{}, err
	}
	changed, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedNPCMindset, error) {
		current, err := c.load(ctx, id, revision)
		if err != nil {
			return ChangedNPCMindset{}, err
		}
		changed, err := c.decideAndSave(ctx, current, &npcmindset.SetCriterionWeightsCommand{Weights: criterionWeights, ChangedBy: changedBy})
		if err != nil {
			return ChangedNPCMindset{}, err
		}
		return changedNPCMindset(changed), nil
	})
	return changed, mapNPCMindsetError(err)
}

// 行動の種類とその重み。種類は application 層の入力として文字列で受け、ここで domain の値にする（presentation 層に domain を見せないため）
type ActionWeightInput struct {
	Action string
	Weight int
}

type CriterionWeightInput struct {
	Criterion string
	Weight    int
}

func parseActionWeights(inputs []ActionWeightInput) (npcmindset.ActionWeights, error) {
	invalid := input.Errors{}
	weights := make(npcmindset.ActionWeights, len(inputs))
	for i, in := range inputs {
		field := fmt.Sprintf("weights.%d", i)
		action, err := matchaction.NewAction(in.Action)
		if err != nil {
			invalid = append(invalid, &input.InvalidFormat{Field: field})
			continue
		}
		if _, seen := weights[action]; seen {
			invalid = append(invalid, &input.Duplicated{Field: field})
			continue
		}
		weight, err := npcmindset.NewWeight(in.Weight)
		if err != nil {
			invalid = append(invalid, &input.OutOfRange{Field: field + ".weight", Min: npcmindset.MinWeight, Max: npcmindset.MaxWeight})
			// 重みの誤りでも種類は使ったことにする（同じ種類の重複と、足りない種類を二重に報告しないため）
			weights[action] = 0
			continue
		}
		weights[action] = weight
	}
	if len(invalid) == 0 && len(weights) != len(matchaction.Actions()) {
		invalid = append(invalid, &input.Required{Field: "weights"})
	}
	if len(invalid) > 0 {
		return nil, invalid
	}
	return weights, nil
}

func parseCriterionWeights(inputs []CriterionWeightInput) (npcmindset.CriterionWeights, error) {
	invalid := input.Errors{}
	weights := make(npcmindset.CriterionWeights, len(inputs))
	for i, in := range inputs {
		field := fmt.Sprintf("weights.%d", i)
		criterion, err := npcmindset.NewCriterion(in.Criterion)
		if err != nil {
			invalid = append(invalid, &input.InvalidFormat{Field: field})
			continue
		}
		if _, seen := weights[criterion]; seen {
			invalid = append(invalid, &input.Duplicated{Field: field})
			continue
		}
		weight, err := npcmindset.NewWeight(in.Weight)
		if err != nil {
			invalid = append(invalid, &input.OutOfRange{Field: field + ".weight", Min: npcmindset.MinWeight, Max: npcmindset.MaxWeight})
			weights[criterion] = 0
			continue
		}
		weights[criterion] = weight
	}
	if len(invalid) == 0 && len(weights) != len(npcmindset.Criteria()) {
		invalid = append(invalid, &input.Required{Field: "weights"})
	}
	if len(invalid) > 0 {
		return nil, invalid
	}
	return weights, nil
}

// 公開中にする（使用不可からの公開に戻すのも同じ）。重みは登録のときから全種類を持ち、範囲も書くときに確かめているので、
// 公開するとき確かめることは無い。すでに公開中なら変えずに成功する（revision が古ければ ErrRevisionConflict）。ほかのエラーは Rename と同じ
func (c *NPCMindsetHandler) Publish(ctx context.Context, id uuid.UUID, revision int64) (ChangedNPCMindset, error) {
	publishedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedNPCMindset{}, ErrNotOperator
	}
	published, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedNPCMindset, error) {
		current, err := c.load(ctx, id, revision)
		if err != nil {
			return ChangedNPCMindset{}, err
		}
		_, changing, err := current.Status.Publish()
		if err != nil {
			return ChangedNPCMindset{}, err
		}
		if !changing {
			return changedNPCMindset(current), nil
		}
		published, err := c.decideAndSave(ctx, current, &npcmindset.PublishCommand{PublishedBy: publishedBy})
		if err != nil {
			return ChangedNPCMindset{}, err
		}
		return changedNPCMindset(published), nil
	})
	return published, mapNPCMindsetError(err)
}

// 使用不可にする。すでに使用不可なら変えずに成功する。準備中なら ErrInvalidNPCMindsetStatusTransition。ほかのエラーは Rename と同じ
func (c *NPCMindsetHandler) Retire(ctx context.Context, id uuid.UUID, revision int64) (ChangedNPCMindset, error) {
	retiredBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedNPCMindset{}, ErrNotOperator
	}
	retired, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedNPCMindset, error) {
		current, err := c.load(ctx, id, revision)
		if err != nil {
			return ChangedNPCMindset{}, err
		}
		retired, err := c.decideAndSave(ctx, current, &npcmindset.RetireCommand{RetiredBy: retiredBy})
		if err != nil {
			return ChangedNPCMindset{}, err
		}
		return changedNPCMindset(retired), nil
	})
	return retired, mapNPCMindsetError(err)
}

// 準備中のものだけ消せる。削除のイベントと一緒に識別子を消し、名前を使えるようにする。準備中でなければ ErrNPCMindsetNotDraft。
// ほかのエラーは Rename と同じ
func (c *NPCMindsetHandler) Delete(ctx context.Context, id uuid.UUID, revision int64) error {
	deletedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ErrNotOperator
	}
	_, err := rdb.Transact(ctx, func(ctx context.Context) (struct{}, error) {
		current, err := c.load(ctx, id, revision)
		if err != nil {
			return struct{}{}, err
		}
		if _, err := c.decideAndSave(ctx, current, &npcmindset.DeleteCommand{DeletedBy: deletedBy}); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, c.identifier.Delete(ctx, current.ID)
	})
	return mapNPCMindsetError(err)
}

// revision を Load の直後に比べる理由は PresetDeckHandler.load と同じ
func (c *NPCMindsetHandler) load(ctx context.Context, publicID uuid.UUID, revision int64) (npcmindset.NPCMindset, error) {
	id, err := c.identifier.Resolve(ctx, publicID)
	if errors.Is(err, domain.ErrEntityNotFound) {
		return npcmindset.NPCMindset{}, ErrNPCMindsetNotFound
	}
	if err != nil {
		return npcmindset.NPCMindset{}, err
	}
	state, err := c.eventStore.Load(ctx, id)
	if err != nil {
		return npcmindset.NPCMindset{}, err
	}
	current, ok := state.(npcmindset.NPCMindset)
	if !ok {
		return npcmindset.NPCMindset{}, ErrNPCMindsetNotFound
	}
	if current.Version != revision {
		return npcmindset.NPCMindset{}, ErrRevisionConflict
	}
	return current, nil
}

// 状態を変えない操作（同じ名前・公開中にもう一度公開する等）は版が進まない
func (c *NPCMindsetHandler) decideAndSave(ctx context.Context, state npcmindset.State, cmd npcmindset.Command) (npcmindset.NPCMindset, error) {
	events, err := npcmindset.Decide(state, cmd, c.now())
	if err != nil {
		return npcmindset.NPCMindset{}, err
	}
	if err := c.eventStore.Save(ctx, events); err != nil {
		return npcmindset.NPCMindset{}, err
	}
	for _, event := range events {
		if state, err = npcmindset.Evolve(state, event); err != nil {
			return npcmindset.NPCMindset{}, err
		}
	}
	current, _ := state.(npcmindset.NPCMindset)
	return current, nil
}

func changedNPCMindset(current npcmindset.NPCMindset) ChangedNPCMindset {
	return ChangedNPCMindset{ID: current.PublicID, Revision: current.Version}
}

func newNPCMindsetName(field, value string) (npcmindset.Name, input.Error) {
	name, err := npcmindset.NewName(value)
	switch {
	case errors.Is(err, npcmindset.ErrNameEmpty):
		return "", &input.Required{Field: field}
	case errors.Is(err, npcmindset.ErrNameTooLong):
		return "", &input.TooLong{Field: field, Max: npcmindset.MaxNameLength}
	case err != nil:
		return "", &input.InvalidFormat{Field: field}
	}
	return name, nil
}

// presentation 層にドメインのエラーを見せないため、application 層のエラーに変える
func mapNPCMindsetError(err error) error {
	switch {
	case errors.Is(err, npcmindset.ErrNotDraft):
		return ErrNPCMindsetNotDraft
	case errors.Is(err, npcmindset.ErrInvalidStatusTransition):
		return ErrInvalidNPCMindsetStatusTransition
	}
	return revisionConflict(err)
}
