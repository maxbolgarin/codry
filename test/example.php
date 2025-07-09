<?php

/**
 * User management system in PHP
 */

namespace UserManagement;

use DateTime;
use Exception;
use InvalidArgumentException;
use PDO;

// Constants
const MAX_USERS = 1000;
const DEFAULT_STATUS = 'active';
const CACHE_TTL = 300;

// Global variables
$userCount = 0;
$isInitialized = false;

/**
 * User status enumeration
 */
class UserStatus
{
    const ACTIVE = 'active';
    const INACTIVE = 'inactive';
    const SUSPENDED = 'suspended';
    const DELETED = 'deleted';

    public static function getValidStatuses(): array
    {
        return [
            self::ACTIVE,
            self::INACTIVE,
            self::SUSPENDED,
            self::DELETED
        ];
    }

    public static function isValid(string $status): bool
    {
        return in_array($status, self::getValidStatuses());
    }
}

/**
 * Permission enumeration
 */
class Permission
{
    const READ = 'read';
    const WRITE = 'write';
    const DELETE = 'delete';
    const ADMIN = 'admin';

    public static function getValidPermissions(): array
    {
        return [self::READ, self::WRITE, self::DELETE, self::ADMIN];
    }
}

/**
 * Custom exception classes
 */
class UserNotFoundException extends Exception
{
    public function __construct(int $id)
    {
        parent::__construct("User with ID {$id} not found");
    }
}

class ValidationException extends Exception
{
    public function __construct(string $field, string $message)
    {
        parent::__construct("Validation error for {$field}: {$message}");
    }
}

class DuplicateEmailException extends Exception
{
    public function __construct(string $email)
    {
        parent::__construct("User with email {$email} already exists");
    }
}

/**
 * User repository interface
 */
interface UserRepositoryInterface
{
    public function findById(int $id): ?User;
    public function save(User $user): User;
    public function delete(int $id): bool;
    public function findByEmail(string $email): ?User;
    public function findAll(): array;
    public function findByStatus(string $status): array;
}

/**
 * User service interface
 */
interface UserServiceInterface
{
    public function getUser(int $id): User;
    public function createUser(CreateUserRequest $request): User;
    public function updateUser(int $id, UpdateUserRequest $request): User;
    public function deleteUser(int $id): bool;
    public function listUsers(?string $status = null): array;
    public function searchUsers(string $query): array;
}

/**
 * Cacheable trait for adding caching functionality
 */
trait Cacheable
{
    private array $cache = [];

    protected function getCacheKey(string $method, array $args): string
    {
        return $method . '_' . md5(serialize($args));
    }

    protected function getFromCache(string $key)
    {
        if (isset($this->cache[$key])) {
            $item = $this->cache[$key];
            if (time() - $item['timestamp'] < CACHE_TTL) {
                return $item['data'];
            }
            unset($this->cache[$key]);
        }
        return null;
    }

    protected function setCache(string $key, $data): void
    {
        $this->cache[$key] = [
            'data' => $data,
            'timestamp' => time()
        ];
    }

    protected function clearCache(): void
    {
        $this->cache = [];
    }
}

/**
 * Loggable trait for adding logging functionality
 */
trait Loggable
{
    private array $logs = [];

    protected function log(string $level, string $message, array $context = []): void
    {
        $this->logs[] = [
            'timestamp' => new DateTime(),
            'level' => $level,
            'message' => $message,
            'context' => $context
        ];
    }

    protected function info(string $message, array $context = []): void
    {
        $this->log('INFO', $message, $context);
    }

    protected function error(string $message, array $context = []): void
    {
        $this->log('ERROR', $message, $context);
    }

    public function getLogs(): array
    {
        return $this->logs;
    }
}

/**
 * Create user request class
 */
class CreateUserRequest
{
    public string $name;
    public string $email;
    public array $permissions;
    public array $metadata;

    public function __construct(
        string $name = '',
        string $email = '',
        array $permissions = [],
        array $metadata = []
    ) {
        $this->name = $name;
        $this->email = $email;
        $this->permissions = $permissions;
        $this->metadata = $metadata;
    }

    public function validate(): void
    {
        if (empty($this->name)) {
            throw new ValidationException('name', 'Name is required');
        }

        if (empty($this->email) || !filter_var($this->email, FILTER_VALIDATE_EMAIL)) {
            throw new ValidationException('email', 'Valid email is required');
        }

        foreach ($this->permissions as $permission) {
            if (!in_array($permission, Permission::getValidPermissions())) {
                throw new ValidationException('permissions', "Invalid permission: {$permission}");
            }
        }
    }
}

/**
 * Update user request class
 */
class UpdateUserRequest
{
    public ?string $name = null;
    public ?string $email = null;
    public ?string $status = null;
    public ?array $permissions = null;
    public ?array $metadata = null;

    public function hasUpdates(): bool
    {
        return $this->name !== null ||
               $this->email !== null ||
               $this->status !== null ||
               $this->permissions !== null ||
               $this->metadata !== null;
    }
}

/**
 * User entity class
 */
class User
{
    private int $id;
    private string $name;
    private string $email;
    private string $status;
    private DateTime $createdAt;
    private ?DateTime $lastLoginAt = null;
    private array $permissions;
    private array $metadata;

    public function __construct(
        int $id = 0,
        string $name = '',
        string $email = '',
        string $status = UserStatus::ACTIVE,
        array $permissions = [],
        array $metadata = []
    ) {
        $this->id = $id;
        $this->name = $name;
        $this->email = $email;
        $this->status = $status;
        $this->createdAt = new DateTime();
        $this->permissions = $permissions;
        $this->metadata = $metadata;
    }

    // Getters
    public function getId(): int
    {
        return $this->id;
    }

    public function getName(): string
    {
        return $this->name;
    }

    public function getEmail(): string
    {
        return $this->email;
    }

    public function getStatus(): string
    {
        return $this->status;
    }

    public function getCreatedAt(): DateTime
    {
        return $this->createdAt;
    }

    public function getLastLoginAt(): ?DateTime
    {
        return $this->lastLoginAt;
    }

    public function getPermissions(): array
    {
        return $this->permissions;
    }

    public function getMetadata(): array
    {
        return $this->metadata;
    }

    // Setters
    public function setId(int $id): void
    {
        $this->id = $id;
    }

    public function setName(string $name): void
    {
        $this->name = $name;
    }

    public function setEmail(string $email): void
    {
        $this->email = $email;
    }

    public function setStatus(string $status): void
    {
        if (!UserStatus::isValid($status)) {
            throw new InvalidArgumentException("Invalid status: {$status}");
        }
        $this->status = $status;
    }

    public function setPermissions(array $permissions): void
    {
        $this->permissions = $permissions;
    }

    public function setMetadata(array $metadata): void
    {
        $this->metadata = $metadata;
    }

    // Business methods
    public function activate(): void
    {
        $this->status = UserStatus::ACTIVE;
    }

    public function deactivate(): void
    {
        $this->status = UserStatus::INACTIVE;
    }

    public function suspend(): void
    {
        $this->status = UserStatus::SUSPENDED;
    }

    public function hasPermission(string $permission): bool
    {
        return in_array($permission, $this->permissions) || 
               in_array(Permission::ADMIN, $this->permissions);
    }

    public function addPermission(string $permission): void
    {
        if (!$this->hasPermission($permission)) {
            $this->permissions[] = $permission;
        }
    }

    public function removePermission(string $permission): void
    {
        $this->permissions = array_values(array_filter(
            $this->permissions,
            fn($p) => $p !== $permission
        ));
    }

    public function updateLastLogin(): void
    {
        $this->lastLoginAt = new DateTime();
    }

    public function getDisplayName(): string
    {
        return "{$this->name} ({$this->email})";
    }

    public function isActive(): bool
    {
        return $this->status === UserStatus::ACTIVE;
    }

    public function toArray(): array
    {
        return [
            'id' => $this->id,
            'name' => $this->name,
            'email' => $this->email,
            'status' => $this->status,
            'created_at' => $this->createdAt->format('Y-m-d H:i:s'),
            'last_login_at' => $this->lastLoginAt?->format('Y-m-d H:i:s'),
            'permissions' => $this->permissions,
            'metadata' => $this->metadata
        ];
    }

    public static function fromArray(array $data): self
    {
        $user = new self(
            $data['id'] ?? 0,
            $data['name'] ?? '',
            $data['email'] ?? '',
            $data['status'] ?? UserStatus::ACTIVE,
            $data['permissions'] ?? [],
            $data['metadata'] ?? []
        );

        if (isset($data['created_at'])) {
            $user->createdAt = new DateTime($data['created_at']);
        }

        if (isset($data['last_login_at'])) {
            $user->lastLoginAt = new DateTime($data['last_login_at']);
        }

        return $user;
    }

    public function __toString(): string
    {
        return "User[{$this->id}]: {$this->getDisplayName()}";
    }
}

/**
 * Memory-based user repository implementation
 */
class MemoryUserRepository implements UserRepositoryInterface
{
    use Loggable;

    private array $users = [];
    private int $nextId = 1;

    public function findById(int $id): ?User
    {
        return $this->users[$id] ?? null;
    }

    public function save(User $user): User
    {
        if ($user->getId() === 0) {
            $user->setId($this->nextId++);
        }

        $this->users[$user->getId()] = $user;
        $this->info("Saved user", ['id' => $user->getId(), 'name' => $user->getName()]);

        return $user;
    }

    public function delete(int $id): bool
    {
        if (isset($this->users[$id])) {
            unset($this->users[$id]);
            $this->info("Deleted user", ['id' => $id]);
            return true;
        }

        return false;
    }

    public function findByEmail(string $email): ?User
    {
        foreach ($this->users as $user) {
            if ($user->getEmail() === $email) {
                return $user;
            }
        }

        return null;
    }

    public function findAll(): array
    {
        return array_values($this->users);
    }

    public function findByStatus(string $status): array
    {
        return array_values(array_filter(
            $this->users,
            fn(User $user) => $user->getStatus() === $status
        ));
    }

    public function count(): int
    {
        return count($this->users);
    }

    public function clear(): void
    {
        $this->users = [];
        $this->nextId = 1;
    }
}

/**
 * Database user repository implementation
 */
class DatabaseUserRepository implements UserRepositoryInterface
{
    use Loggable;

    private PDO $pdo;

    public function __construct(PDO $pdo)
    {
        $this->pdo = $pdo;
        $this->initializeTable();
    }

    private function initializeTable(): void
    {
        $sql = "
            CREATE TABLE IF NOT EXISTS users (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                name TEXT NOT NULL,
                email TEXT NOT NULL UNIQUE,
                status TEXT NOT NULL,
                created_at TEXT NOT NULL,
                last_login_at TEXT,
                permissions TEXT,
                metadata TEXT
            )
        ";

        $this->pdo->exec($sql);
    }

    public function findById(int $id): ?User
    {
        $stmt = $this->pdo->prepare('SELECT * FROM users WHERE id = ?');
        $stmt->execute([$id]);
        $data = $stmt->fetch(PDO::FETCH_ASSOC);

        return $data ? $this->mapToUser($data) : null;
    }

    public function save(User $user): User
    {
        if ($user->getId() === 0) {
            return $this->insert($user);
        } else {
            return $this->update($user);
        }
    }

    private function insert(User $user): User
    {
        $sql = "
            INSERT INTO users (name, email, status, created_at, last_login_at, permissions, metadata)
            VALUES (?, ?, ?, ?, ?, ?, ?)
        ";

        $stmt = $this->pdo->prepare($sql);
        $stmt->execute([
            $user->getName(),
            $user->getEmail(),
            $user->getStatus(),
            $user->getCreatedAt()->format('Y-m-d H:i:s'),
            $user->getLastLoginAt()?->format('Y-m-d H:i:s'),
            json_encode($user->getPermissions()),
            json_encode($user->getMetadata())
        ]);

        $user->setId((int) $this->pdo->lastInsertId());
        return $user;
    }

    private function update(User $user): User
    {
        $sql = "
            UPDATE users 
            SET name = ?, email = ?, status = ?, last_login_at = ?, permissions = ?, metadata = ?
            WHERE id = ?
        ";

        $stmt = $this->pdo->prepare($sql);
        $stmt->execute([
            $user->getName(),
            $user->getEmail(),
            $user->getStatus(),
            $user->getLastLoginAt()?->format('Y-m-d H:i:s'),
            json_encode($user->getPermissions()),
            json_encode($user->getMetadata()),
            $user->getId()
        ]);

        return $user;
    }

    public function delete(int $id): bool
    {
        $stmt = $this->pdo->prepare('DELETE FROM users WHERE id = ?');
        $stmt->execute([$id]);
        return $stmt->rowCount() > 0;
    }

    public function findByEmail(string $email): ?User
    {
        $stmt = $this->pdo->prepare('SELECT * FROM users WHERE email = ?');
        $stmt->execute([$email]);
        $data = $stmt->fetch(PDO::FETCH_ASSOC);

        return $data ? $this->mapToUser($data) : null;
    }

    public function findAll(): array
    {
        $stmt = $this->pdo->query('SELECT * FROM users ORDER BY created_at');
        $users = [];

        while ($data = $stmt->fetch(PDO::FETCH_ASSOC)) {
            $users[] = $this->mapToUser($data);
        }

        return $users;
    }

    public function findByStatus(string $status): array
    {
        $stmt = $this->pdo->prepare('SELECT * FROM users WHERE status = ? ORDER BY created_at');
        $stmt->execute([$status]);
        $users = [];

        while ($data = $stmt->fetch(PDO::FETCH_ASSOC)) {
            $users[] = $this->mapToUser($data);
        }

        return $users;
    }

    private function mapToUser(array $data): User
    {
        $userData = [
            'id' => (int) $data['id'],
            'name' => $data['name'],
            'email' => $data['email'],
            'status' => $data['status'],
            'created_at' => $data['created_at'],
            'last_login_at' => $data['last_login_at'],
            'permissions' => json_decode($data['permissions'], true) ?? [],
            'metadata' => json_decode($data['metadata'], true) ?? []
        ];

        return User::fromArray($userData);
    }
}

/**
 * User service implementation
 */
class UserService implements UserServiceInterface
{
    use Cacheable, Loggable;

    private UserRepositoryInterface $repository;

    public function __construct(UserRepositoryInterface $repository)
    {
        $this->repository = $repository;
    }

    public function getUser(int $id): User
    {
        $cacheKey = $this->getCacheKey('getUser', [$id]);
        $cached = $this->getFromCache($cacheKey);

        if ($cached !== null) {
            return $cached;
        }

        $user = $this->repository->findById($id);
        if ($user === null) {
            throw new UserNotFoundException($id);
        }

        $this->setCache($cacheKey, $user);
        return $user;
    }

    public function createUser(CreateUserRequest $request): User
    {
        $request->validate();

        $existing = $this->repository->findByEmail($request->email);
        if ($existing !== null) {
            throw new DuplicateEmailException($request->email);
        }

        $user = new User(
            0,
            $request->name,
            $request->email,
            UserStatus::ACTIVE,
            $request->permissions,
            $request->metadata
        );

        $savedUser = $this->repository->save($user);

        global $userCount;
        $userCount++;

        $this->info("Created user", ['id' => $savedUser->getId(), 'name' => $savedUser->getName()]);
        return $savedUser;
    }

    public function updateUser(int $id, UpdateUserRequest $request): User
    {
        if (!$request->hasUpdates()) {
            throw new InvalidArgumentException('No updates provided');
        }

        $user = $this->getUser($id);

        if ($request->name !== null) {
            $user->setName($request->name);
        }

        if ($request->email !== null) {
            if (!filter_var($request->email, FILTER_VALIDATE_EMAIL)) {
                throw new ValidationException('email', 'Invalid email format');
            }
            $user->setEmail($request->email);
        }

        if ($request->status !== null) {
            $user->setStatus($request->status);
        }

        if ($request->permissions !== null) {
            $user->setPermissions($request->permissions);
        }

        if ($request->metadata !== null) {
            $user->setMetadata(array_merge($user->getMetadata(), $request->metadata));
        }

        $updatedUser = $this->repository->save($user);
        $this->clearCache();

        return $updatedUser;
    }

    public function deleteUser(int $id): bool
    {
        $user = $this->getUser($id);
        $success = $this->repository->delete($id);

        if ($success) {
            global $userCount;
            $userCount--;
            $this->clearCache();
            $this->info("Deleted user", ['id' => $id, 'name' => $user->getName()]);
        }

        return $success;
    }

    public function listUsers(?string $status = null): array
    {
        if ($status !== null) {
            return $this->repository->findByStatus($status);
        }

        return $this->repository->findAll();
    }

    public function searchUsers(string $query): array
    {
        $users = $this->repository->findAll();
        $lowerQuery = strtolower($query);

        return array_filter($users, function (User $user) use ($lowerQuery) {
            return str_contains(strtolower($user->getName()), $lowerQuery) ||
                   str_contains(strtolower($user->getEmail()), $lowerQuery);
        });
    }
}

/**
 * User manager class
 */
class UserManager
{
    use Loggable;

    private UserServiceInterface $service;

    public function __construct(UserServiceInterface $service)
    {
        $this->service = $service;
    }

    public function bulkCreateUsers(array $requests): array
    {
        $createdUsers = [];

        foreach ($requests as $request) {
            try {
                if ($request instanceof CreateUserRequest) {
                    $user = $this->service->createUser($request);
                    $createdUsers[] = $user;
                } else {
                    $this->error("Invalid request type", ['request' => $request]);
                }
            } catch (Exception $e) {
                $this->error("Failed to create user", [
                    'request' => $request,
                    'error' => $e->getMessage()
                ]);
            }
        }

        return $createdUsers;
    }

    public function getUserStats(): array
    {
        $users = $this->service->listUsers();

        $stats = [
            'total' => count($users),
            'active' => 0,
            'inactive' => 0,
            'suspended' => 0,
            'deleted' => 0
        ];

        foreach ($users as $user) {
            $stats[$user->getStatus()]++;
        }

        return $stats;
    }

    public function exportUsers(string $format = 'json'): string
    {
        $users = $this->service->listUsers();
        $userData = array_map(fn(User $user) => $user->toArray(), $users);

        switch ($format) {
            case 'json':
                return json_encode($userData, JSON_PRETTY_PRINT);

            case 'csv':
                if (empty($userData)) {
                    return '';
                }

                $output = fopen('php://temp', 'w');
                fputcsv($output, array_keys($userData[0]));

                foreach ($userData as $row) {
                    fputcsv($output, $row);
                }

                rewind($output);
                $csv = stream_get_contents($output);
                fclose($output);

                return $csv;

            default:
                throw new InvalidArgumentException("Unsupported format: {$format}");
        }
    }

    public function importUsers(string $data, string $format = 'json'): array
    {
        switch ($format) {
            case 'json':
                $userData = json_decode($data, true);
                break;

            case 'csv':
                $userData = $this->parseCsv($data);
                break;

            default:
                throw new InvalidArgumentException("Unsupported format: {$format}");
        }

        $requests = [];
        foreach ($userData as $data) {
            $requests[] = new CreateUserRequest(
                $data['name'] ?? '',
                $data['email'] ?? '',
                $data['permissions'] ?? [],
                $data['metadata'] ?? []
            );
        }

        return $this->bulkCreateUsers($requests);
    }

    private function parseCsv(string $data): array
    {
        $lines = explode("\n", trim($data));
        if (empty($lines)) {
            return [];
        }

        $headers = str_getcsv(array_shift($lines));
        $result = [];

        foreach ($lines as $line) {
            if (trim($line) !== '') {
                $values = str_getcsv($line);
                $result[] = array_combine($headers, $values);
            }
        }

        return $result;
    }
}

// Utility functions
function validateEmail(string $email): bool
{
    return filter_var($email, FILTER_VALIDATE_EMAIL) !== false;
}

function generateRandomUser(): CreateUserRequest
{
    $names = ['Alice', 'Bob', 'Charlie', 'Diana', 'Eve', 'Frank'];
    $domains = ['example.com', 'test.org', 'demo.net'];

    $name = $names[array_rand($names)] . ' ' . ucfirst(substr(md5(rand()), 0, 6));
    $email = strtolower(str_replace(' ', '.', $name)) . '@' . $domains[array_rand($domains)];

    return new CreateUserRequest($name, $email, [Permission::READ]);
}

function createUserService(string $type = 'memory', ?PDO $pdo = null): UserServiceInterface
{
    switch ($type) {
        case 'memory':
            $repository = new MemoryUserRepository();
            break;

        case 'database':
            if ($pdo === null) {
                throw new InvalidArgumentException('PDO instance required for database repository');
            }
            $repository = new DatabaseUserRepository($pdo);
            break;

        default:
            throw new InvalidArgumentException("Unknown repository type: {$type}");
    }

    return new UserService($repository);
}

function initializeUserSystem(): void
{
    global $isInitialized, $userCount;

    $isInitialized = true;
    $userCount = 0;

    echo "User management system initialized\n";
}

// Main execution
function main(): void
{
    try {
        initializeUserSystem();

        $service = createUserService('memory');
        $manager = new UserManager($service);

        // Create test users
        $requests = [
            new CreateUserRequest('Alice Johnson', 'alice@example.com', [Permission::READ, Permission::WRITE]),
            new CreateUserRequest('Bob Smith', 'bob@example.com', [Permission::READ]),
            new CreateUserRequest('Charlie Admin', 'charlie@example.com', [Permission::ADMIN])
        ];

        $createdUsers = $manager->bulkCreateUsers($requests);
        echo "Created " . count($createdUsers) . " users\n";

        // Get stats
        $stats = $manager->getUserStats();
        echo "User statistics:\n";
        foreach ($stats as $key => $value) {
            echo "  {$key}: {$value}\n";
        }

        // Export users
        $exportedData = $manager->exportUsers('json');
        echo "Exported users:\n{$exportedData}\n";

    } catch (Exception $e) {
        echo "Error: " . $e->getMessage() . "\n";
        echo $e->getTraceAsString() . "\n";
    }
}

// Execute if this is the main file
if (basename(__FILE__) === basename($_SERVER['SCRIPT_NAME'])) {
    main();
} 