// MongoDB 建库建表脚本
// 使用方法: mongosh < init_mongodb.js

// ==================== 数据库操作 ====================
const dbName = "aiwork";  // 与 config.Mongo.Database 对应

// ==================== 1. user 用户表 ====================
db.getSiblingDB(dbName).createCollection("user");
db.getSiblingDB(dbName).user.createIndex({ "name": 1 }, { unique: true });
db.getSiblingDB(dbName).user.createIndex({ "status": 1 });

// ==================== 2. department 部门表 ====================
db.getSiblingDB(dbName).createCollection("department");
db.getSiblingDB(dbName).department.createIndex({ "name": 1 });
db.getSiblingDB(dbName).department.createIndex({ "parentId": 1 });
db.getSiblingDB(dbName).department.createIndex({ "parent_path": 1 });

// ==================== 3. department_user 部门用户关联表 ====================
db.getSiblingDB(dbName).createCollection("department_user");
db.getSiblingDB(dbName).department_user.createIndex({ "departmentId": 1 });
db.getSiblingDB(dbName).department_user.createIndex({ "userId": 1 });
db.getSiblingDB(dbName).department_user.createIndex(
    { "departmentId": 1, "userId": 1 },
    { unique: true }
);

// ==================== 4. todo 待办任务表 ====================
db.getSiblingDB(dbName).createCollection("todo");
db.getSiblingDB(dbName).todo.createIndex({ "creatorId": 1 });
db.getSiblingDB(dbName).todo.createIndex({ "todo_status": 1 });
db.getSiblingDB(dbName).todo.createIndex({ "deadlineAt": 1 });
db.getSiblingDB(dbName).todo.createIndex({ "createAt": -1 });

// ==================== 5. user_todo 用户待办关联表 ====================
db.getSiblingDB(dbName).createCollection("user_todo");
db.getSiblingDB(dbName).user_todo.createIndex({ "userId": 1 });
db.getSiblingDB(dbName).user_todo.createIndex({ "todoId": 1 });
db.getSiblingDB(dbName).user_todo.createIndex({ "todoStatus": 1 });
db.getSiblingDB(dbName).user_todo.createIndex(
    { "userId": 1, "todoId": 1 },
    { unique: true }
);

// ==================== 6. approval 审批表 ====================
db.getSiblingDB(dbName).createCollection("approval");
db.getSiblingDB(dbName).approval.createIndex({ "userId": 1 });
db.getSiblingDB(dbName).approval.createIndex({ "no": 1 }, { unique: true });
db.getSiblingDB(dbName).approval.createIndex({ "type": 1 });
db.getSiblingDB(dbName).approval.createIndex({ "status": 1 });
db.getSiblingDB(dbName).approval.createIndex({ "createAt": -1 });
db.getSiblingDB(dbName).approval.createIndex({ "approvalId": 1 });

// ==================== 7. chat_log 聊天记录表 ====================
db.getSiblingDB(dbName).createCollection("chat_log");
db.getSiblingDB(dbName).chat_log.createIndex({ "conversationId": 1 });
db.getSiblingDB(dbName).chat_log.createIndex({ "sendId": 1 });
db.getSiblingDB(dbName).chat_log.createIndex({ "recvId": 1 });
db.getSiblingDB(dbName).chat_log.createIndex({ "sendTime": -1 });
db.getSiblingDB(dbName).chat_log.createIndex(
    { "recvId": 1, "sendTime": -1 }
);

// ==================== 初始化管理员账号 ====================
// 密码: 123456 (bcrypt hash)
db.getSiblingDB(dbName).user.insertOne({
    name: "root",
    password: "$2a$10$ddIvqt7U6zNA9poys.FNCuEZTJY6V.axWy4P7A44TuT9KBegGZlD6",
    status: 0,
    isSystem: true,
    createAt: ISODate().getTime(),
    updateAt: ISODate().getTime()
});

print("========================================");
print("MongoDB 数据库初始化完成!");
print("数据库名: " + dbName);
print("集合列表:");
print("  - user (用户表)");
print("  - department (部门表)");
print("  - department_user (部门用户关联表)");
print("  - todo (待办任务表)");
print("  - user_todo (用户待办关联表)");
print("  - approval (审批表)");
print("  - chat_log (聊天记录表)");
print("========================================");
