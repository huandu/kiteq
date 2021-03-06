package store

import (
	"fmt"
	"kiteq/protocol"
)

//用于持久化的messageEntity
type MessageEntity struct {
	Header *protocol.Header
	body   []byte //序列化后的消息
	//-----------------
	msgType uint8 //消息类型

	messageId    string
	topic        string //topic
	messageType  string //messageType
	publishGroup string //发布的groupId
	commit       bool   //是否已提交
	expiredTime  int64  //过期时间

	kiteQServer string // 当前的处理kiteqserver地址

	failGroupTags []string //投递失败的分组tags
	// succGroupTags   []string // 投递成功的分组
	nextDeliverTime int64 //下一次投递的时间

}

func (self *MessageEntity) String() string {
	return fmt.Sprintf("id:%s topic:%s commit:%t body:%s", self.messageId, self.topic, self.commit, string(self.body))
}

func (self *MessageEntity) GetBody() []byte {
	return self.body
}

//创建stringmessage
func NewStringMessageEntity(msg *protocol.StringMessage) *MessageEntity {
	entity := &MessageEntity{
		Header:    msg.GetHeader(),
		messageId: msg.GetHeader().GetMessageId(),

		topic:        msg.GetHeader().GetTopic(),
		publishGroup: msg.GetHeader().GetGroupId(),
		messageType:  msg.GetHeader().GetMessageType(),
		commit:       msg.GetHeader().GetCommit(),
		expiredTime:  msg.GetHeader().GetExpiredTime(),
		//消息种类
		msgType: protocol.CMD_STRING_MESSAGE,
		body:    []byte(msg.GetBody())}
	return entity

}

//创建bytesmessage的实体
func NewBytesMessageEntity(msg *protocol.BytesMessage) *MessageEntity {
	entity := &MessageEntity{
		Header:       msg.GetHeader(),
		messageId:    msg.GetHeader().GetMessageId(),
		topic:        msg.GetHeader().GetTopic(),
		publishGroup: msg.GetHeader().GetGroupId(),
		messageType:  msg.GetHeader().GetMessageType(),
		commit:       msg.GetHeader().GetCommit(),
		expiredTime:  msg.GetHeader().GetExpiredTime(),

		//消息种类
		msgType: protocol.CMD_BYTES_MESSAGE,
		body:    msg.GetBody()}

	return entity
}

//kitestore存储
type IKiteStore interface {
	Query(messageId string) *MessageEntity
	Save(entity *MessageEntity) bool
	Commit(messageId string) bool
	Rollback(messageId string) bool
	UpdateEntity(entity *MessageEntity) bool
	Delete(messageId string) bool
}
