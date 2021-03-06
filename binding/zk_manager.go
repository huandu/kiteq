package binding

import (
	"github.com/blackbeans/zk"
	"log"
	_ "net"
	"strings"
	"time"
)

const (
	KITEQ        = "/kiteq"
	KITEQ_SERVER = KITEQ + "/server" // 临时节点 # /kiteq/server/${topic}/ip:port
	KITEQ_PUB    = KITEQ + "/pub"    // 临时节点 # /kiteq/pub/${topic}/${groupId}/ip:port
	KITEQ_SUB    = KITEQ + "/sub"    // 持久订阅/或者临时订阅 # /kiteq/sub/${topic}/${groupId}-bind/#$data(bind)
)

type ZKManager struct {
	session *zk.Session
}

type ZkEvent zk.EventType

const (
	Created ZkEvent = 1 // From Exists, Get NodeCreated (1),
	Deleted ZkEvent = 2 // From Exists, Get	NodeDeleted (2),
	Changed ZkEvent = 3 // From Exists, Get NodeDataChanged (3),
	Child   ZkEvent = 4 // From Children NodeChildrenChanged (4)
)

//每个watcher
type IWatcher interface {
	EventNotify(path string, eventType ZkEvent, binds []*Binding)
	ChildWatcher(path string, childNode []string)
}

type Watcher struct {
	watcher   IWatcher
	zkwatcher chan zk.Event
}

//创建一个watcher
func NewWatcher(watcher IWatcher) *Watcher {
	zkwatcher := make(chan zk.Event, 10)
	return &Watcher{watcher: watcher, zkwatcher: zkwatcher}
}

func NewZKManager(zkhosts string) *ZKManager {
	if len(zkhosts) <= 0 {
		log.Println("使用默认zkhosts！|localhost:2181\n")
		zkhosts = "localhost:2181"
	} else {
		log.Printf("使用zkhosts:[%s]！\n", zkhosts)
	}

	conf := &zk.Config{Addrs: strings.Split(zkhosts, ","), Timeout: 5 * time.Second}

	ss, err := zk.Dial(conf)
	if nil != err {
		panic("连接zk失败..." + err.Error())
		return nil
	}

	exist, _, err := ss.Exists(KITEQ, nil)
	if nil != err {
		panic("无法创建KITEQ " + err.Error())
	}

	if !exist {
		resp, err := ss.Create(KITEQ, nil, zk.CreatePersistent, zk.AclOpen)
		if nil != err {
			panic("NewZKManager|CREATE ROOT PATH|FAIL|" + KITEQ + "|" + err.Error())
		} else {
			log.Printf("NewZKManager|CREATE ROOT PATH|SUCC|%s", resp)
		}
	}

	return &ZKManager{session: ss}
}

//发布topic对应的server
func (self *ZKManager) PublishQServer(hostport string, topics []string) error {

	for _, topic := range topics {
		qpath := KITEQ_SERVER + "/" + topic
		path, err := self.registePath(qpath, hostport, zk.CreateEphemeral, nil)
		if nil != err {
			log.Printf("ZKManager|PublishQServer|FAIL|%s|%s/%s\n", err, qpath, hostport)
			return err
		}
		log.Printf("ZKManager|PublishQServer|SUCC|%s\n", path)
	}

	return nil
}

//发布可以使用的topic类型的publisher
func (self *ZKManager) PublishTopics(topics []string, groupId string, hostport string) error {

	for _, topic := range topics {
		pubPath := KITEQ_PUB + "/" + topic + "/" + groupId
		path, err := self.registePath(pubPath, hostport, zk.CreateEphemeral, nil)
		if nil != err {
			log.Printf("ZKManager|PublishTopic|FAIL|%s|%s/%s\n", err, pubPath, hostport)
			return err
		}
		log.Printf("ZKManager|PublishTopic|SUCC|%s\n", path)
	}
	return nil
}

//发布订阅关系
func (self *ZKManager) PublishBindings(groupId string, bindings []*Binding) error {

	//按topic分组
	groupBind := make(map[string][]*Binding, 10)
	for _, b := range bindings {
		g, ok := groupBind[b.Topic]
		if !ok {
			g = make([]*Binding, 0, 2)
		}
		b.GroupId = groupId
		g = append(g, b)
		groupBind[b.Topic] = g
	}

	for topic, binds := range groupBind {
		data, err := MarshalBinds(binds)
		if nil != err {
			log.Printf("ZKManager|PublishBindings|MarshalBind|FAIL|%s|%s|%t\n", err, groupId, binds)
			return err
		}

		//如果为非持久订阅则直接注册临时节点
		createType := zk.CreatePersistent
		// if !binding.Persistent {
		// 	createType = zk.CreateEphemeral
		// }

		path := KITEQ_SUB + "/" + topic
		//注册对应topic的groupId //注册订阅信息
		succpath, err := self.registePath(path, groupId+"-bind", createType, data)
		if nil != err {
			log.Printf("ZKManager|PublishTopic|Bind|FAIL|%s|%s/%s\n", err, path, binds)
			return err
		} else {
			log.Printf("ZKManager|PublishTopic|Bind|SUCC|%s|%s\n", succpath, binds)
		}
	}
	return nil
}

//注册当前进程节点
func (self *ZKManager) registePath(path string, childpath string, createType zk.CreateType, data []byte) (string, error) {
	err := self.traverseCreatePath(path, nil, zk.CreatePersistent)
	if nil == err {
		err := self.innerCreatePath(path+"/"+childpath, data, createType)
		if nil != err {
			log.Printf("ZKManager|registePath|CREATE CHILD|FAIL|%s|%s\n", err, path+"/"+childpath)
			return "", err
		} else {
			return path + "/" + childpath, nil
		}
	}
	return "", err

}

func (self *ZKManager) traverseCreatePath(path string, data []byte, createType zk.CreateType) error {
	split := strings.Split(path, "/")[1:]
	tmppath := "/"
	for i, v := range split {
		tmppath += v
		// log.Printf("ZKManager|traverseCreatePath|%s\n", tmppath)
		if i >= len(split)-1 {
			break
		}
		err := self.innerCreatePath(tmppath, nil, zk.CreatePersistent)
		if nil != err {
			log.Printf("ZKManager|traverseCreatePath|FAIL|%s\n", err)
			return err
		}
		tmppath += "/"

	}

	//对叶子节点创建及添加数据
	return self.innerCreatePath(tmppath, data, createType)
}

//内部创建节点的方法
func (self *ZKManager) innerCreatePath(tmppath string, data []byte, createType zk.CreateType) error {
	exist, _, err := self.session.Exists(tmppath, nil)
	if nil == err && !exist {
		_, err := self.session.Create(tmppath, data, createType, zk.AclOpen)
		if nil != err {
			log.Printf("ZKManager|innerCreatePath|FAIL|%s|%s\n", err, tmppath)
			return err
		}

		//做一下校验等待
		for i := 0; i < 5; i++ {
			exist, _, _ = self.session.Exists(tmppath, nil)
			if !exist {
				time.Sleep(time.Duration(i*100) * time.Millisecond)
			} else {
				break
			}
		}

		return err
	} else if nil != err {
		log.Printf("ZKManager|innerCreatePath|FAIL|%s\n", err)
		return err
	} else if nil != data {
		//存在该节点，推送新数据
		_, err := self.session.Set(tmppath, data, -1)
		if nil != err {
			log.Printf("ZKManager|innerCreatePath|PUSH DATA|FAIL|%s|%s|%s\n", err, tmppath, string(data))
			return err
		}
	}
	return nil
}

//获取QServer并添加watcher
func (self *ZKManager) GetQServerAndWatch(topic string, nwatcher *Watcher) ([]string, error) {

	path := KITEQ_SERVER + "/" + topic

	exist, _, _ := self.session.Exists(path, nwatcher.zkwatcher)
	if !exist {
		return []string{}, nil
	}

	//获取topic下的所有qserver
	children, _, err := self.session.Children(path, nwatcher.zkwatcher)
	if nil != err {
		log.Printf("ZKManager|GetQServerAndWatch|FAIL|%s\n", path)
		return nil, err
	}

	//增加监听
	self.addWatch(path, nwatcher)
	return children, nil
}

//获取订阅关系并添加watcher
func (self *ZKManager) GetBindAndWatch(topic string, nwatcher *Watcher) ([]*Binding, error) {

	path := KITEQ_SUB + "/" + topic

	exist, _, _ := self.session.Exists(path, nwatcher.zkwatcher)
	if !exist {
		return []*Binding{}, nil
	}

	//获取topic下的所有qserver
	groupIds, _, err := self.session.Children(path, nwatcher.zkwatcher)
	if nil != err {
		log.Printf("ZKManager|GetBindAndWatch|GroupID|FAIL|%s|%s\n", err, path)
		return nil, err
	}

	//加入对订阅topic节点的变化
	self.addWatch(path, nwatcher)

	hps := make([]*Binding, 0, len(groupIds))
	//获取topic对应的所有groupId下的订阅关系
	for _, groupId := range groupIds {
		tmppath := path + "/" + groupId
		binds, err := self.getBindData(tmppath, nwatcher.zkwatcher)
		if nil != err {
			continue
		}
		hps = append(hps, binds...)
	}

	return hps, nil
}

//获取绑定对象的数据
func (self *ZKManager) getBindData(path string, zkwatcher chan zk.Event) ([]*Binding, error) {

	bindData, _, err := self.session.Get(path, zkwatcher)
	if nil != err {
		log.Printf("ZKManager|getBindData|Binding|FAIL|%s|%s\n", err, path)
		return nil, err
	}

	// log.Printf("ZKManager|getBindData|Binding|SUCC|%s|%s\n", path, string(bindData))

	binding, err := UmarshalBinds(bindData)
	if nil != err {
		log.Printf("ZKManager|getBindData|UmarshalBind|FAIL|%s|%s|%s\n", err, path, string(bindData))

	}
	return binding, err
}

func (self *ZKManager) addWatch(path string, nwatcher *Watcher) {

	//监听数据变更
	go func() {
		for {
			//根据zk的文档 watcher机制是无法保证可靠的，其次需要在每次处理完watcher后要重新注册watcher
			change := <-nwatcher.zkwatcher
			switch change.Type {
			case zk.Created:
				self.session.Exists(path, nwatcher.zkwatcher)
				nwatcher.watcher.EventNotify(path, Created, nil)
			case zk.Deleted:
				self.session.Exists(path, nwatcher.zkwatcher)
				nwatcher.watcher.EventNotify(path, Deleted, nil)

			case zk.Changed:
				split := strings.Split(path, "/")
				//如果不是bind级别的变更则忽略
				if len(split) < 5 || strings.LastIndex(split[4], "-bind") <= 0 {
					self.session.Exists(path, nwatcher.zkwatcher)
					continue
				}

				//获取一下数据
				binding, err := self.getBindData(path, nwatcher.zkwatcher)
				if nil != err {
					log.Printf("ZKManager|addWatch|Changed|Get DATA|FAIL|%s|%s\n", err, path)
					//忽略
					continue
				}
				nwatcher.watcher.EventNotify(path, Changed, binding)

			case zk.Child:
				//子节点发生变更，则获取全新的子节点
				childnodes, _, err := self.session.Children(path, nwatcher.zkwatcher)
				if nil != err {
					log.Printf("ZKManager|addWatch|Child|%s|%s\n", err, path)
				} else {
					log.Printf("ZKManager|%s|child's changed| %s\n", path, childnodes)
					nwatcher.watcher.ChildWatcher(path, childnodes)
				}
			}
		}
		log.Printf("ZKManager|addWatch|FAIL|out of wacher range ! [%s]\n", path)
	}()
}

func (self *ZKManager) Close() {
	self.session.Close()
}
