package stgbroker

import (
	"git.oschina.net/cloudzone/smartgo/stgbroker/client"
	"git.oschina.net/cloudzone/smartgo/stgbroker/client/rebalance"
	"git.oschina.net/cloudzone/smartgo/stgbroker/mqtrace"
	"git.oschina.net/cloudzone/smartgo/stgbroker/out"
	"git.oschina.net/cloudzone/smartgo/stgbroker/stats"
	"git.oschina.net/cloudzone/smartgo/stgcommon"
	"git.oschina.net/cloudzone/smartgo/stgcommon/constant"
	"git.oschina.net/cloudzone/smartgo/stgcommon/logger"
	"git.oschina.net/cloudzone/smartgo/stgcommon/protocol"
	"git.oschina.net/cloudzone/smartgo/stgcommon/utils/timeutil"
	"git.oschina.net/cloudzone/smartgo/stgnet/remoting"
	"time"
)

type BrokerController struct {
	BrokerConfig stgcommon.BrokerConfig
	// TODO
	// nettyServerConfig
	// nettyClientConfig
	// messageStoreConfig
	ConfigDataVersion         *stgcommon.DataVersion
	ConsumerOffsetManager     *ConsumerOffsetManager
	ConsumerManager           *client.ConsumerManager
	ProducerManager           *client.ProducerManager
	ClientHousekeepingService *ClientHouseKeepingService
	// DefaultTransactionCheckExecuter
	PullMessageProcessor      *PullMessageProcessor
	PullRequestHoldService    *PullRequestHoldService
	Broker2Client             *Broker2Client
	SubscriptionGroupManager  *SubscriptionGroupManager
	ConsumerIdsChangeListener rebalance.ConsumerIdsChangeListener
	// RebalanceLockManager
	BrokerOuterAPI *out.BrokerOuterAPI
	// ScheduledExecutorService
	SlaveSynchronize *SlaveSynchronize
	//MessageStore       *stgstorelog.MessageStore
	RemotingServer     *remoting.DefalutRemotingServer
	TopicConfigManager *TopicConfigManager
	// SendMessageExecutor ExecutorService
	// PullMessageExecutor ExecutorService
	// adminBrokerExecutor ExecutorService
	// clientManageExecutor ExecutorService
	UpdateMasterHAServerAddrPeriodically bool
	// brokerStats BrokerStats
	FilterServerManager *FilterServerManager
	brokerStatsManager  *stats.BrokerStatsManager
	StoreHost           string
	// configFile
	ConfigFile             *string
	sendMessageHookList    []mqtrace.SendMessageHook
	consumeMessageHookList []mqtrace.ConsumeMessageHook
}

func NewBrokerController(brokerConfig stgcommon.BrokerConfig, /* nettyServerConfig NettyServerConfig,
   nettyClientConfig NettyClientConfig , //
   messageStoreConfig MessageStoreConfig */) *BrokerController {
	var brokerController = new(BrokerController)
	brokerController.BrokerConfig = brokerConfig
	// TODO nettyServerConfig
	// TODO nettyServerConfig
	// TODO messageStoreConfig
	brokerController.ConfigDataVersion = stgcommon.NewDataVersion()
	brokerController.ConsumerOffsetManager = NewConsumerOffsetManager(brokerController)
	brokerController.UpdateMasterHAServerAddrPeriodically = false
	brokerController.TopicConfigManager = NewTopicConfigManager(brokerController)
	brokerController.PullMessageProcessor = NewPullMessageProcessor(brokerController)
	brokerController.PullRequestHoldService = NewPullRequestHoldService(brokerController)
	// TODO defaultTransactionCheckExecuter
	brokerController.ConsumerIdsChangeListener = NewDefaultConsumerIdsChangeListener(brokerController)
	brokerController.ConsumerManager = client.NewConsumerManager(brokerController.ConsumerIdsChangeListener)
	brokerController.ProducerManager = client.NewProducerManager()
	brokerController.ClientHousekeepingService = NewClientHousekeepingService(brokerController)
	brokerController.Broker2Client = NewBroker2Clientr(brokerController)
	brokerController.SubscriptionGroupManager = NewSubscriptionGroupManager(brokerController)
	brokerController.BrokerOuterAPI = out.NewBrokerOuterAPI()
	brokerController.FilterServerManager = NewFilterServerManager(brokerController)

	if brokerController.BrokerConfig.NamesrvAddr != "" {
		brokerController.BrokerOuterAPI.UpdateNameServerAddressList(brokerController.BrokerConfig.NamesrvAddr)
		logger.Info("user specfied name server address: {}", brokerController.BrokerConfig.NamesrvAddr)
	}

	brokerController.SlaveSynchronize = NewSlaveSynchronize(brokerController)
	brokerController.brokerStatsManager = stats.NewBrokerStatsManager(brokerController.BrokerConfig.BrokerClusterName)
	return brokerController
}

// GetBrokerAddr 获得broker的Addr
// Author rongzhihong
// Since 2017/9/5
func (bc *BrokerController) GetBrokerAddr() string {
	return bc.BrokerConfig.BrokerIP1 + ":" + bc.RemotingServer.GetListenPort()
}

func (bc *BrokerController) Initialize() bool {
	result := true
	result = result && bc.TopicConfigManager.Load()
	result = result && bc.SubscriptionGroupManager.Load()
	result = result && bc.ConsumerOffsetManager.Load()

	if result {
		// TODO messageStore
	}

	// TODO result = result && this.messageStore.load();
	if !result {
		return result
	}

	bc.RemotingServer = remoting.NewDefalutRemotingServer("0.0.0.0", 10911)
	bc.StoreHost = bc.BrokerConfig.BrokerIP1 + ":" + bc.RemotingServer.GetListenPort()

	// 注册服务
	bc.registerProcessor()
	// TODO 统计

	// 定时写入ConsumerOffset文件
	consumerOffsetPersistTicker := timeutil.NewTicker(bc.BrokerConfig.FlushConsumerOffsetInterval, 1000*10)
	go consumerOffsetPersistTicker.Do(func(tm time.Time) {
		bc.ConsumerOffsetManager.configManagerExt.Persist()
	})

	// 扫描数据被删除了的topic，offset记录也对应删除
	scanUnsubscribedTopicTicker := timeutil.NewTicker(5*1000, 1000*10)
	go scanUnsubscribedTopicTicker.Do(func(tm time.Time) {
		bc.ConsumerOffsetManager.ScanUnsubscribedTopic()
	})

	// 如果namesrv不为空则更新namesrv地址
	if bc.BrokerConfig.NamesrvAddr != "" {
		bc.BrokerOuterAPI.UpdateNameServerAddressList(bc.BrokerConfig.NamesrvAddr)
	} else {
		// 更新
		if bc.BrokerConfig.FetchNamesrvAddrByAddressServer {
			FetchNameServerAddrTicker := timeutil.NewTicker(60*1000*2, 1000*10)
			go FetchNameServerAddrTicker.Do(func(tm time.Time) {
				bc.BrokerOuterAPI.FetchNameServerAddr()
			})
		}
	}

	// TODO messageStoreConfig
	return result

}

func (bc *BrokerController) Shutdown() {
	bc.RemotingServer.Shutdown()
	if bc.PullRequestHoldService != nil {
		bc.PullRequestHoldService.Shutdown()
	}
	if bc.ClientHousekeepingService != nil {
		bc.ClientHousekeepingService.Shutdown()
	}
	if bc.FilterServerManager != nil {
		bc.FilterServerManager.Shutdown()
	}
}

func (bc *BrokerController) Start() {
	if bc.PullRequestHoldService != nil {
		bc.PullRequestHoldService.Start()
	}
	if bc.ClientHousekeepingService != nil {
		bc.ClientHousekeepingService.Start()
	}
	if bc.FilterServerManager != nil {
		bc.FilterServerManager.Start()
	}
	bc.RemotingServer.Start()
}

func (bc *BrokerController) RegisterBrokerAll(checkOrderConfig bool, oneway bool) {
	topicConfigWrapper := bc.TopicConfigManager.buildTopicConfigSerializeWrapper()
	if !constant.IsWriteable(bc.BrokerConfig.BrokerPermission) || !constant.IsReadable(bc.BrokerConfig.BrokerPermission) {
		topicConfigTable := topicConfigWrapper.TopicConfigTable
		bc.TopicConfigManager.TopicConfigSerializeWrapper.TopicConfigTable.Foreach(func(k string, topicConfig *stgcommon.TopicConfig) {
			topicConfig.Perm = bc.BrokerConfig.BrokerPermission
		})
		topicConfigWrapper.TopicConfigTable = topicConfigTable
	}
	registerBrokerResult := bc.BrokerOuterAPI.RegisterBrokerAll(
		bc.BrokerConfig.BrokerClusterName,
		bc.GetBrokerAddr(),
		bc.BrokerConfig.BrokerName,
		bc.getHAServerAddr(),
		bc.BrokerConfig.BrokerId,
		topicConfigWrapper,
		oneway,
		nil)

	if registerBrokerResult != nil {
		if bc.UpdateMasterHAServerAddrPeriodically && registerBrokerResult.HaServerAddr != "" {
			// TODO this.messageStore.updateHaMasterAddress(registerBrokerResult.getHaServerAddr());
		}

		bc.SlaveSynchronize.masterAddr = registerBrokerResult.MasterAddr

		if checkOrderConfig {
			bc.TopicConfigManager.updateOrderTopicConfig(registerBrokerResult.KvTable)
		}
	}
}
func (bc *BrokerController) getHAServerAddr() string {
	// TODO:
	return ""
}

func (bc *BrokerController) addDeleteTopicTask() {
	// 定时写入ConsumerOffset文件
	addDeleteTopicTaskTicker := timeutil.NewTicker(bc.BrokerConfig.FlushConsumerOffsetInterval, 1000*60*5)
	go addDeleteTopicTaskTicker.Do(func(tm time.Time) {
		bc.ConsumerOffsetManager.configManagerExt.Persist()
	})
}

// registerProcessor 注册提供服务
// Author gaoyanlei
// Since 2017/8/25
func (bc *BrokerController) registerProcessor() {

	clientProcessor := NewClientManageProcessor(bc)
	// 心跳
	bc.RemotingServer.RegisterProcessor(protocol.HEART_BEAT, clientProcessor)
	// 注销client
	bc.RemotingServer.RegisterProcessor(protocol.UNREGISTER_CLIENT, clientProcessor)
	// 获取Consumer
	bc.RemotingServer.RegisterProcessor(protocol.GET_CONSUMER_LIST_BY_GROUP, clientProcessor)
	// 查询Consumer offset
	bc.RemotingServer.RegisterProcessor(protocol.QUERY_CONSUMER_OFFSET, clientProcessor)
	// 更新Consumer offset
	bc.RemotingServer.RegisterProcessor(protocol.UPDATE_CONSUMER_OFFSET, clientProcessor)

	adminBrokerProcessor := NewAdminBrokerProcessor(bc)
	// 更新创建topic
	bc.RemotingServer.RegisterProcessor(protocol.UPDATE_AND_CREATE_TOPIC, adminBrokerProcessor)
	// 删除topic
	bc.RemotingServer.RegisterProcessor(protocol.DELETE_TOPIC_IN_BROKER, adminBrokerProcessor)
	// 获取最大offset
	bc.RemotingServer.RegisterProcessor(protocol.GET_MAX_OFFSET, adminBrokerProcessor)

	sendMessageProcessor := NewSendMessageProcessor(bc)
	sendMessageProcessor.RegisterSendMessageHook(bc.sendMessageHookList)
	// 未优化过发送消息
	bc.RemotingServer.RegisterProcessor(protocol.SEND_MESSAGE, sendMessageProcessor)
	// 优化过发送消息
	bc.RemotingServer.RegisterProcessor(protocol.SEND_MESSAGE_V2, sendMessageProcessor)
	// 消费失败消息
	bc.RemotingServer.RegisterProcessor(protocol.CONSUMER_SEND_MSG_BACK, sendMessageProcessor)

	pullMessageProcessor := NewPullMessageProcessor(bc)
	// 拉取消息
	bc.RemotingServer.RegisterProcessor(protocol.PULL_MESSAGE, pullMessageProcessor)
	pullMessageProcessor.RegisterConsumeMessageHook(bc.consumeMessageHookList)

	// TODO QueryMessageProcessor
	// TODO EndTransactionProcessor
}

// getConfigDataVersion 获得数据配置版本号
// Author rongzhihong
// Since 2017/9/8
func (bc *BrokerController) getConfigDataVersion() string {
	return bc.ConfigDataVersion.ToJson()
}

// RegisterSendMessageHook 注册发送消息的回调
// Author rongzhihong
// Since 2017/9/11
func (bc *BrokerController) RegisterSendMessageHook(hook mqtrace.SendMessageHook) {
	bc.sendMessageHookList = append(bc.sendMessageHookList, hook)
	logger.Info("register SendMessageHook Hook, %s", hook.HookName())
}

// RegisterSendMessageHook 注册消费消息的回调
// Author rongzhihong
// Since 2017/9/11
func (bc *BrokerController) RegisterConsumeMessageHook(hook mqtrace.ConsumeMessageHook) {
	bc.consumeMessageHookList = append(bc.consumeMessageHookList, hook)
	logger.Info("register ConsumeMessageHook Hook, %s", hook.HookName())
}
