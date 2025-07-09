/**
 * User management system in TypeScript
 */

// Type definitions and interfaces
interface IUser {
    id: number;
    name: string;
    email: string;
    status: UserStatus;
    createdAt: Date;
    metadata?: Record<string, any>;
}

interface IUserRepository<T = IUser> {
    getById(id: number): Promise<T | null>;
    save(user: T): Promise<T>;
    delete(id: number): Promise<boolean>;
    findByEmail(email: string): Promise<T | null>;
    listAll(): Promise<T[]>;
}

interface IUserService {
    getUser(id: number): Promise<User>;
    createUser(userData: CreateUserRequest): Promise<User>;
    updateUser(id: number, updates: Partial<IUser>): Promise<User>;
    deleteUser(id: number): Promise<boolean>;
    listUsers(status?: UserStatus): Promise<User[]>;
    searchUsers(query: string): Promise<User[]>;
}

// Type aliases and enums
type UserId = number;
type UserEmail = string;
type UserName = string;

enum UserStatus {
    ACTIVE = 'active',
    INACTIVE = 'inactive',
    SUSPENDED = 'suspended',
    DELETED = 'deleted'
}

enum Permission {
    READ = 'read',
    WRITE = 'write',
    DELETE = 'delete',
    ADMIN = 'admin'
}

// Generic types
type ApiResponse<T> = {
    data: T;
    status: 'success' | 'error';
    message?: string;
    timestamp: Date;
};

type Repository<T, K = number> = {
    findById(id: K): Promise<T | null>;
    save(entity: T): Promise<T>;
    remove(id: K): Promise<boolean>;
};

// Request/Response types
interface CreateUserRequest {
    name: UserName;
    email: UserEmail;
    permissions?: Permission[];
    metadata?: Record<string, any>;
}

interface UpdateUserRequest {
    name?: UserName;
    email?: UserEmail;
    status?: UserStatus;
    permissions?: Permission[];
}

interface UserResponse extends IUser {
    permissions: Permission[];
    lastLoginAt?: Date;
}

// Constants
const MAX_USERS = 1000;
const DEFAULT_PERMISSIONS: Permission[] = [Permission.READ];
const USER_CACHE_TTL = 300000; // 5 minutes

// Global variables
let userCount: number = 0;
let isInitialized: boolean = false;

// Custom decorators
function LogMethod(target: any, propertyName: string, descriptor: PropertyDescriptor) {
    const method = descriptor.value;
    
    descriptor.value = function (...args: any[]) {
        console.log(`Calling ${propertyName} with arguments:`, args);
        const result = method.apply(this, args);
        console.log(`${propertyName} returned:`, result);
        return result;
    };
}

function Cache(ttl: number = 300000) {
    return function (target: any, propertyName: string, descriptor: PropertyDescriptor) {
        const method = descriptor.value;
        const cache = new Map<string, { data: any; timestamp: number }>();
        
        descriptor.value = function (...args: any[]) {
            const key = JSON.stringify(args);
            const cached = cache.get(key);
            const now = Date.now();
            
            if (cached && (now - cached.timestamp) < ttl) {
                return cached.data;
            }
            
            const result = method.apply(this, args);
            cache.set(key, { data: result, timestamp: now });
            return result;
        };
    };
}

function Validate(validator: (value: any) => boolean, message: string) {
    return function (target: any, propertyName: string) {
        let value = target[propertyName];
        
        Object.defineProperty(target, propertyName, {
            get: () => value,
            set: (newValue) => {
                if (!validator(newValue)) {
                    throw new Error(message);
                }
                value = newValue;
            },
            enumerable: true,
            configurable: true
        });
    };
}

// Custom error classes
class UserNotFoundError extends Error {
    constructor(id: UserId) {
        super(`User with ID ${id} not found`);
        this.name = 'UserNotFoundError';
    }
}

class ValidationError extends Error {
    constructor(field: string, message: string) {
        super(`Validation error for ${field}: ${message}`);
        this.name = 'ValidationError';
    }
}

class DuplicateEmailError extends Error {
    constructor(email: UserEmail) {
        super(`User with email ${email} already exists`);
        this.name = 'DuplicateEmailError';
    }
}

// Utility functions
const validateEmail = (email: string): boolean => {
    const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
    return emailRegex.test(email);
};

const generateId = (): UserId => Math.floor(Math.random() * 1000000);

const isValidUserStatus = (status: string): status is UserStatus => {
    return Object.values(UserStatus).includes(status as UserStatus);
};

const createApiResponse = <T>(data: T, status: 'success' | 'error' = 'success', message?: string): ApiResponse<T> => ({
    data,
    status,
    message,
    timestamp: new Date()
});

// User class
class User implements IUser {
    public readonly id: UserId;
    public name: UserName;
    public email: UserEmail;
    public status: UserStatus;
    public readonly createdAt: Date;
    public metadata: Record<string, any>;
    public permissions: Permission[];
    public lastLoginAt?: Date;

    constructor(
        id: UserId,
        name: UserName,
        email: UserEmail,
        status: UserStatus = UserStatus.ACTIVE,
        permissions: Permission[] = DEFAULT_PERMISSIONS
    ) {
        this.id = id;
        this.name = name;
        this.email = email;
        this.status = status;
        this.createdAt = new Date();
        this.metadata = {};
        this.permissions = permissions;
    }

    public activate(): void {
        this.status = UserStatus.ACTIVE;
    }

    public deactivate(): void {
        this.status = UserStatus.INACTIVE;
    }

    public suspend(): void {
        this.status = UserStatus.SUSPENDED;
    }

    public hasPermission(permission: Permission): boolean {
        return this.permissions.includes(permission) || this.permissions.includes(Permission.ADMIN);
    }

    public addPermission(permission: Permission): void {
        if (!this.hasPermission(permission)) {
            this.permissions.push(permission);
        }
    }

    public removePermission(permission: Permission): void {
        this.permissions = this.permissions.filter(p => p !== permission);
    }

    public updateLastLogin(): void {
        this.lastLoginAt = new Date();
    }

    public toJSON(): UserResponse {
        return {
            id: this.id,
            name: this.name,
            email: this.email,
            status: this.status,
            createdAt: this.createdAt,
            metadata: this.metadata,
            permissions: this.permissions,
            lastLoginAt: this.lastLoginAt
        };
    }

    public static fromJSON(data: UserResponse): User {
        const user = new User(data.id, data.name, data.email, data.status, data.permissions);
        user.metadata = data.metadata || {};
        user.lastLoginAt = data.lastLoginAt;
        return user;
    }
}

// Repository implementation
class MemoryUserRepository implements IUserRepository<User> {
    private users: Map<UserId, User> = new Map();
    private nextId: UserId = 1;

    async getById(id: UserId): Promise<User | null> {
        return this.users.get(id) || null;
    }

    async save(user: User): Promise<User> {
        if (!user.id) {
            (user as any).id = this.nextId++;
        }
        this.users.set(user.id, user);
        return user;
    }

    async delete(id: UserId): Promise<boolean> {
        return this.users.delete(id);
    }

    async findByEmail(email: UserEmail): Promise<User | null> {
        for (const user of this.users.values()) {
            if (user.email === email) {
                return user;
            }
        }
        return null;
    }

    async listAll(): Promise<User[]> {
        return Array.from(this.users.values());
    }

    public size(): number {
        return this.users.size;
    }

    public clear(): void {
        this.users.clear();
        this.nextId = 1;
    }
}

// Service class
class UserService implements IUserService {
    private cache: Map<UserId, { user: User; timestamp: number }> = new Map();

    constructor(
        private repository: IUserRepository<User>,
        private logger?: Console
    ) {
        this.logger = logger || console;
    }

    async getUser(id: UserId): Promise<User> {
        const user = await this.repository.getById(id);
        if (!user) {
            throw new UserNotFoundError(id);
        }
        return user;
    }

    async createUser(userData: CreateUserRequest): Promise<User> {
        if (!validateEmail(userData.email)) {
            throw new ValidationError('email', 'Invalid email format');
        }

        const existingUser = await this.repository.findByEmail(userData.email);
        if (existingUser) {
            throw new DuplicateEmailError(userData.email);
        }

        const user = new User(
            generateId(),
            userData.name,
            userData.email,
            UserStatus.ACTIVE,
            userData.permissions || DEFAULT_PERMISSIONS
        );

        if (userData.metadata) {
            user.metadata = userData.metadata;
        }

        const savedUser = await this.repository.save(user);
        userCount++;

        this.logger?.log(`Created user: ${savedUser.name} (${savedUser.email})`);
        return savedUser;
    }

    async updateUser(id: UserId, updates: Partial<IUser>): Promise<User> {
        const user = await this.getUser(id);

        if (updates.name !== undefined) user.name = updates.name;
        if (updates.email !== undefined) {
            if (!validateEmail(updates.email)) {
                throw new ValidationError('email', 'Invalid email format');
            }
            user.email = updates.email;
        }
        if (updates.status !== undefined) user.status = updates.status;
        if (updates.metadata !== undefined) user.metadata = { ...user.metadata, ...updates.metadata };

        const updatedUser = await this.repository.save(user);
        this.invalidateCache(id);

        return updatedUser;
    }

    async deleteUser(id: UserId): Promise<boolean> {
        const user = await this.getUser(id);
        const success = await this.repository.delete(id);

        if (success) {
            userCount--;
            this.invalidateCache(id);
            this.logger?.log(`Deleted user: ${user.name} (${user.email})`);
        }

        return success;
    }

    async listUsers(status?: UserStatus): Promise<User[]> {
        const users = await this.repository.listAll();
        
        if (status) {
            return users.filter(user => user.status === status);
        }
        
        return users.sort((a, b) => a.createdAt.getTime() - b.createdAt.getTime());
    }

    async searchUsers(query: string): Promise<User[]> {
        const users = await this.repository.listAll();
        const lowerQuery = query.toLowerCase();

        return users.filter(user =>
            user.name.toLowerCase().includes(lowerQuery) ||
            user.email.toLowerCase().includes(lowerQuery)
        );
    }

    private invalidateCache(id: UserId): void {
        this.cache.delete(id);
    }
}

// Manager class with generics
class UserManager<T extends IUser = User> {
    constructor(private service: IUserService) {}

    async bulkCreateUsers(userDataList: CreateUserRequest[]): Promise<ApiResponse<User[]>> {
        const createdUsers: User[] = [];
        const errors: string[] = [];

        for (const userData of userDataList) {
            try {
                const user = await this.service.createUser(userData);
                createdUsers.push(user);
            } catch (error) {
                errors.push(`Failed to create user ${userData.name}: ${error.message}`);
            }
        }

        return createApiResponse(createdUsers, errors.length > 0 ? 'error' : 'success');
    }

    async getUserStats(): Promise<ApiResponse<{
        total: number;
        active: number;
        inactive: number;
        suspended: number;
    }>> {
        const users = await this.service.listUsers();
        
        const stats = {
            total: users.length,
            active: users.filter(u => u.status === UserStatus.ACTIVE).length,
            inactive: users.filter(u => u.status === UserStatus.INACTIVE).length,
            suspended: users.filter(u => u.status === UserStatus.SUSPENDED).length
        };

        return createApiResponse(stats);
    }

    async exportUsers(format: 'json' | 'csv' = 'json'): Promise<string> {
        const users = await this.service.listUsers();
        const userData = users.map(user => user.toJSON());

        if (format === 'json') {
            return JSON.stringify(userData, null, 2);
        } else {
            // Simple CSV implementation
            const headers = Object.keys(userData[0] || {});
            const csvRows = [
                headers.join(','),
                ...userData.map(row => 
                    headers.map(header => JSON.stringify(row[header as keyof UserResponse])).join(',')
                )
            ];
            return csvRows.join('\n');
        }
    }
}

// Factory functions
function createUserRepository(type: 'memory' | 'database' = 'memory'): IUserRepository<User> {
    switch (type) {
        case 'memory':
            return new MemoryUserRepository();
        case 'database':
            throw new Error('Database repository not implemented');
        default:
            throw new Error(`Unknown repository type: ${type}`);
    }
}

function createUserService(repository?: IUserRepository<User>): UserService {
    const repo = repository || createUserRepository('memory');
    return new UserService(repo);
}

// Initialization function
function initializeUserSystem(config: {
    repositoryType?: 'memory' | 'database';
    maxUsers?: number;
    defaultPermissions?: Permission[];
} = {}): { service: UserService; manager: UserManager } {
    isInitialized = true;
    userCount = 0;

    const repository = createUserRepository(config.repositoryType);
    const service = createUserService(repository);
    const manager = new UserManager(service);

    console.log('User management system initialized');
    return { service, manager };
}

// Async main function
async function main(): Promise<void> {
    const { service, manager } = initializeUserSystem();

    // Create test users
    const testUsers: CreateUserRequest[] = [
        { name: 'Alice Johnson', email: 'alice@example.com', permissions: [Permission.READ, Permission.WRITE] },
        { name: 'Bob Smith', email: 'bob@example.com', permissions: [Permission.READ] },
        { name: 'Charlie Admin', email: 'charlie@example.com', permissions: [Permission.ADMIN] }
    ];

    const result = await manager.bulkCreateUsers(testUsers);
    console.log('Bulk create result:', result);

    const stats = await manager.getUserStats();
    console.log('User stats:', stats);

    const exportedData = await manager.exportUsers('json');
    console.log('Exported users:', exportedData);
}

// Module exports
export {
    User,
    UserService,
    UserManager,
    MemoryUserRepository,
    UserStatus,
    Permission,
    createUserRepository,
    createUserService,
    initializeUserSystem,
    type IUser,
    type IUserRepository,
    type IUserService,
    type CreateUserRequest,
    type UpdateUserRequest,
    type UserResponse,
    type ApiResponse
};

// Execute main function
main().catch(console.error); 