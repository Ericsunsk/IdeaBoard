package administration

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgSetRole                  = "role 仅支持 admin 或 user"                   // SetRole @ commands.go
	msgSetRole2                 = "目标用户的业务侧角色无效"                            // SetRole @ commands.go
	msgBan                      = "expires_in_days 不能小于 0"                  // Ban @ commands.go
	msgBan2                     = "expires_in_days 超出支持范围"                  // Ban @ commands.go
	msgBan3                     = "目标用户在身份系统中不存在，已拒绝封禁以避免状态漂移"              // Ban @ commands.go
	msgBan4                     = "身份侧封禁失败"                                 // Ban @ commands.go
	msgBan5                     = "业务侧封禁失败"                                 // Ban @ commands.go
	msgBan6                     = "业务侧封禁失败，且身份侧回滚失败，请按运维流程补偿处理"             // Ban @ commands.go
	msgUnban                    = "目标用户在身份系统中不存在，已拒绝解封以避免状态漂移"              // Unban @ commands.go
	msgUnban2                   = "身份侧解封失败"                                 // Unban @ commands.go
	msgUnban3                   = "业务侧解封失败"                                 // Unban @ commands.go
	msgUnban4                   = "业务侧解封失败，且身份侧回滚失败，请按运维流程补偿处理"             // Unban @ commands.go
	msgRevokeSessions           = "目标用户在身份系统中不存在，无法吊销会话"                    // RevokeSessions @ commands.go
	msgRevokeSessions2          = "身份侧会话吊销失败"                               // RevokeSessions @ commands.go
	msgDelete                   = "存在未结算的 Payment、Refund 或 Dispute，暂不能注销用户" // Delete @ commands.go
	msgDelete2                  = "认证服务未配置"                                 // Delete @ commands.go
	msgDelete3                  = "用户删除流程已安全冻结，可重试以继续收敛"                    // Delete @ commands.go
	msgPromote                  = "目标用户在身份系统中不存在，已拒绝改角色以避免状态漂移"             // Promote @ commands.go
	msgPromote2                 = "同步 Auth 管理员角色失败"                         // Promote @ commands.go
	msgPromote3                 = "业务侧角色更新失败"                               // Promote @ commands.go
	msgPromote4                 = "业务侧角色更新失败，且 Auth 角色回滚失败，请按运维流程补偿处理"      // Promote @ commands.go
	msgDemote                   = "业务侧已取消管理员，但目标用户在身份系统中不存在，请按运维流程核对"       // Demote @ commands.go
	msgDemote2                  = "业务侧已取消管理员，但 Auth 角色同步失败，请重试或按运维流程补偿处理"   // Demote @ commands.go
	msgLoadTarget               = "管理员身份无效"                                 // LoadTarget @ commands.go
	msgLoadTarget2              = "需要管理员权限"                                 // LoadTarget @ commands.go
	msgLoadTarget3              = "用户不存在"                                   // LoadTarget @ commands.go
	msgLoadTarget4              = "读取用户失败"                                  // LoadTarget @ commands.go
	msgLoadTarget5              = "不能操作当前登录管理员自己"                           // LoadTarget @ commands.go
	msgLoadTarget6              = "不能操作管理员账号"                               // LoadTarget @ commands.go
	msgLoadTarget7              = "目标用户缺少 auth_user_id，已拒绝操作以避免状态漂移"        // LoadTarget @ commands.go
	msgNormalizeUserListRequest = "keyword 长度不能超过 64 个字符"                   // NormalizeUserListRequest @ service.go
	msgEnrichUserList           = "用户Idea余额读取服务未配置"                         // EnrichUserList @ service.go
	msgEnrichUserList2          = "读取用户Idea余额失败: %w"                        // EnrichUserList @ service.go
	msgEnrichUserList3          = "读取用户邮箱失败: %w"                            // EnrichUserList @ service.go
	msgEnrichUserList4          = "用户 %d 的Idea余额缺失"                         // EnrichUserList @ service.go
	msgNormalizeUserListStatus  = "status 仅支持 0 或 1"                        // NormalizeUserListStatus @ service.go
	msgUpdateStatus             = "status 仅支持 0 或 1"                        // UpdateStatus @ service.go
	msgUpdateStatus2            = "用户生命周期服务未配置"                             // UpdateStatus @ service.go
	msgUpdateRole               = "role 仅支持 admin 或 user"                   // UpdateRole @ service.go
	msgDeleteUser               = "用户删除依赖未配置"                               // DeleteUser @ service.go
)
