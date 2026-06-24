# Stora Windows 原生桌面客户端开发教程

> 使用 **WinUI 3**（C#）构建高性能、高颜值的 Windows 桌面应用
> 适用人群：编程新手 / 初学者

---

## 📖 目录

1. [准备工作](#1-准备工作)
2. [创建项目](#2-创建项目)
3. [项目结构](#3-项目结构)
4. [构建 API 客户端](#4-构建-api-客户端)
5. [登录界面](#5-登录界面)
6. [主界面 — 文件浏览器](#6-主界面--文件浏览器)
7. [上传与下载](#7-上传与下载)
8. [系统托盘与后台运行](#8-系统托盘与后台运行)
9. [本地服务器模式](#9-本地服务器模式)
10. [打包发布](#10-打包发布)

---

## 1. 准备工作

### 1.1 安装 Visual Studio 2022

1. 去 https://visualstudio.microsoft.com/zh-hans/downloads/ 下载 **Visual Studio 2022 Community**（免费）
2. 运行安装程序，选择以下工作负载（Workloads）：

   ✅ **使用 .NET 的桌面开发** (.NET desktop development)
   ✅ **通用 Windows 平台开发** (Universal Windows Platform development)

   > 在右侧"安装详细信息"中，确保勾选了 **Windows 应用 SDK C# 模板**

3. 点击安装，等待完成（约 10-20 分钟）

### 1.2 安装 Windows 应用 SDK

WinUI 3 是 Windows 应用 SDK 的一部分。Visual Studio 安装后通常自带，但如果模板缺失：

1. 打开 Visual Studio → 扩展 → 管理扩展
2. 搜索 **Windows App SDK** 并安装
3. 或者去 https://learn.microsoft.com/zh-cn/windows/apps/windows-app-sdk/downloads 手动下载

### 1.3 验证安装

打开 Visual Studio，点击"创建新项目"，搜索 **WinUI**：

![WinUI 3 模板](https://learn.microsoft.com/zh-cn/windows/apps/winui/winui3/images/winui-project-templates.png)

你应该看到：
- **Blank App, Packaged (WinUI 3 in Desktop)** — 选择这个！

---

## 2. 创建项目

### 2.1 新建项目

1. 打开 Visual Studio → **创建新项目**
2. 搜索 `WinUI` → 选择 **Blank App, Packaged (WinUI 3 in Desktop)**
3. 项目名称：`StoraDesktop`
4. 位置：选择你的代码目录（建议放在 Stora 项目文件夹旁边）
5. 解决方案名称：`StoraDesktop`
6. 点击 **创建**

### 2.2 项目结构说明

创建完成后，你会看到这样的结构：

```
StoraDesktop/
├── StoraDesktop.sln          # 解决方案文件
├── StoraDesktop/
│   ├── StoraDesktop.csproj   # 项目文件
│   ├── App.xaml / App.xaml.cs # 应用入口
│   ├── MainWindow.xaml / MainWindow.xaml.cs  # 主窗口
│   ├── app.manifest           # Windows 清单
│   └── Package.appxmanifest   # 打包配置
```

---

## 3. 项目结构规划

我们需要为 Stora 的众多功能组织好代码。先创建以下文件夹结构：

```
StoraDesktop/
├── Models/           # 数据模型（对应 API 的 JSON 响应）
├── Services/         # API 调用服务
├── ViewModels/       # UI 逻辑层（MVVM 模式）
├── Views/            # 页面（每个功能一个页面）
├── Controls/         # 自定义控件
├── Helpers/          # 工具函数
└── Converters/       # XAML 值转换器
```

### 3.1 安装 NuGet 包

右键项目 → **管理 NuGet 包** → 安装以下包：

| 包名 | 用途 |
|------|------|
| `CommunityToolkit.Mvvm` | MVVM 工具包（简化数据绑定） |
| `CommunityToolkit.WinUI.UI` | WinUI 3 社区工具包（额外控件） |
| `System.Text.Json` | JSON 解析（一般自带） |
| `Microsoft.Extensions.DependencyInjection` | 依赖注入 |
| `Microsoft.WindowsAppSDK` | WinUI 3 核心（一般已自带） |

在项目根目录，用终端执行：

```powershell
cd StoraDesktop
dotnet add package CommunityToolkit.Mvvm
dotnet add package CommunityToolkit.WinUI.UI
dotnet add package Microsoft.Extensions.DependencyInjection
```

---

## 4. 构建 API 客户端

### 4.1 数据模型（Models）

在 `Models/` 文件夹下创建几个核心模型：

**`Models/AuthModels.cs`** — 认证相关模型：

```csharp
namespace StoraDesktop.Models;

// 登录请求
public class LoginRequest
{
    public string Username { get; set; } = string.Empty;
    public string Password { get; set; } = string.Empty;
}

// 登录响应
public class LoginResponse
{
    public string AccessToken { get; set; } = string.Empty;
    public string RefreshToken { get; set; } = string.Empty;
    public UserInfo User { get; set; } = new();
}

// 用户信息
public class UserInfo
{
    public string Id { get; set; } = string.Empty;
    public string Username { get; set; } = string.Empty;
    public string Email { get; set; } = string.Empty;
    public long StorageUsed { get; set; }
    public long StorageTotal { get; set; }
}
```

**`Models/FileModels.cs`** — 文件相关模型：

```csharp
namespace StoraDesktop.Models;

public class FileItem
{
    public string Id { get; set; } = string.Empty;
    public string Name { get; set; } = string.Empty;
    public string MimeType { get; set; } = string.Empty;
    public long Size { get; set; }
    public string? FolderId { get; set; }
    public bool IsFolder { get; set; }
    public DateTime CreatedAt { get; set; }
    public DateTime UpdatedAt { get; set; }
    public string? ThumbnailUrl { get; set; }
    public bool IsFavorite { get; set; }

    // 文件大小的格式化显示
    public string SizeDisplay => FormatSize(Size);

    private static string FormatSize(long bytes) => bytes switch
    {
        < 1024 => $"{bytes} B",
        < 1024 * 1024 => $"{bytes / 1024.0:F1} KB",
        < 1024 * 1024 * 1024 => $"{bytes / (1024.0 * 1024):F1} MB",
        _ => $"{bytes / (1024.0 * 1024 * 1024):F2} GB"
    };
}

// API 分页响应
public class PaginatedResponse<T>
{
    public List<T> Data { get; set; } = new();
    public int Total { get; set; }
    public int Page { get; set; }
    public int PerPage { get; set; }
    public bool HasMore { get; set; }
}
```

### 4.2 API 服务（Services）

**`Services/StoraApiClient.cs`** — 核心 HTTP 客户端：

```csharp
using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Text;
using System.Text.Json;
using StoraDesktop.Models;

namespace StoraDesktop.Services;

public class StoraApiClient
{
    private readonly HttpClient _httpClient;
    private string? _accessToken;
    private string? _refreshToken;

    // 服务器地址，可在设置中更改
    public string BaseUrl { get; set; } = "http://localhost:9421";

    public StoraApiClient()
    {
        _httpClient = new HttpClient();
    }

    #region 认证

    /// <summary>
    /// 🔐 设置访问令牌
    /// </summary>
    public void SetTokens(string accessToken, string refreshToken)
    {
        _accessToken = accessToken;
        _refreshToken = refreshToken;
        _httpClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", accessToken);
    }

    public void ClearTokens()
    {
        _accessToken = null;
        _refreshToken = null;
        _httpClient.DefaultRequestHeaders.Authorization = null;
    }

    public bool IsAuthenticated => _accessToken != null;

    /// <summary>
    /// 🔑 登录
    /// </summary>
    public async Task<LoginResponse> LoginAsync(string username, string password)
    {
        var form = new FormUrlEncodedContent(new[]
        {
            new KeyValuePair<string, string>("username", username),
            new KeyValuePair<string, string>("password", password)
        });

        var response = await _httpClient.PostAsync($"{BaseUrl}/api/v2/auth/login", form);
        response.EnsureSuccessStatusCode();

        var result = await response.Content.ReadFromJsonAsync<LoginResponse>();
        if (result != null)
        {
            SetTokens(result.AccessToken, result.RefreshToken);
        }
        return result ?? throw new Exception("登录失败：响应为空");
    }

    /// <summary>
    /// 🔄 刷新令牌
    /// </summary>
    public async Task<LoginResponse?> RefreshTokenAsync()
    {
        if (_refreshToken == null) return null;

        var form = new FormUrlEncodedContent(new[]
        {
            new KeyValuePair<string, string>("refresh_token", _refreshToken)
        });

        var response = await _httpClient.PostAsync($"{BaseUrl}/api/v2/auth/refresh", form);
        if (!response.IsSuccessStatusCode) return null;

        var result = await response.Content.ReadFromJsonAsync<LoginResponse>();
        if (result != null)
        {
            SetTokens(result.AccessToken, result.RefreshToken);
        }
        return result;
    }

    /// <summary>
    /// 🚪 登出
    /// </summary>
    public async Task LogoutAsync()
    {
        try
        {
            await _httpClient.PostAsync($"{BaseUrl}/api/v2/auth/logout", null);
        }
        finally
        {
            ClearTokens();
        }
    }

    /// <summary>
    /// 👤 获取当前用户信息
    /// </summary>
    public async Task<UserInfo> GetCurrentUserAsync()
    {
        var response = await _httpClient.GetAsync($"{BaseUrl}/api/v2/auth/me");
        response.EnsureSuccessStatusCode();
        return await response.Content.ReadFromJsonAsync<UserInfo>()
               ?? throw new Exception("获取用户信息失败");
    }

    #endregion

    #region 文件操作

    /// <summary>
    /// 📂 获取文件列表
    /// </summary>
    public async Task<PaginatedResponse<FileItem>> GetFilesAsync(
        string? folderId = null,
        int page = 1,
        int perPage = 50)
    {
        var url = $"{BaseUrl}/api/v2/files?page={page}&per_page={perPage}";
        if (!string.IsNullOrEmpty(folderId))
            url += $"&folder_id={folderId}";

        var response = await _httpClient.GetAsync(url);
        response.EnsureSuccessStatusCode();
        return await response.Content.ReadFromJsonAsync<PaginatedResponse<FileItem>>()
               ?? new PaginatedResponse<FileItem>();
    }

    /// <summary>
    /// ⬆️ 上传文件（小文件）
    /// </summary>
    public async Task<FileItem> UploadFileAsync(Stream fileStream, string fileName, string? folderId = null)
    {
        using var content = new MultipartFormDataContent();
        content.Add(new StreamContent(fileStream), "file", fileName);
        if (!string.IsNullOrEmpty(folderId))
            content.Add(new StringContent(folderId), "folder_id");

        var response = await _httpClient.PostAsync($"{BaseUrl}/api/v2/files/upload", content);
        response.EnsureSuccessStatusCode();
        return await response.Content.ReadFromJsonAsync<FileItem>()
               ?? throw new Exception("上传失败");
    }

    /// <summary>
    /// ⬇️ 下载文件
    /// </summary>
    public async Task<Stream> DownloadFileAsync(string fileId)
    {
        var response = await _httpClient.GetAsync($"{BaseUrl}/api/v2/files/{fileId}/download");
        response.EnsureSuccessStatusCode();
        return await response.Content.ReadAsStreamAsync();
    }

    /// <summary>
    /// 🗑️ 删除文件（移入回收站）
    /// </summary>
    public async Task DeleteFileAsync(string fileId)
    {
        var response = await _httpClient.DeleteAsync($"{BaseUrl}/api/v2/files/{fileId}");
        response.EnsureSuccessStatusCode();
    }

    // ... 更多文件操作可按需添加

    #endregion

    #region 文件夹操作

    /// <summary>
    /// 📁 获取文件夹树
    /// </summary>
    public async Task<List<FileItem>> GetFolderTreeAsync()
    {
        var response = await _httpClient.GetAsync($"{BaseUrl}/api/v2/files/folders/tree");
        response.EnsureSuccessStatusCode();
        return await response.Content.ReadFromJsonAsync<List<FileItem>>()
               ?? new List<FileItem>();
    }

    /// <summary>
    /// 📁 创建文件夹
    /// </summary>
    public async Task<FileItem> CreateFolderAsync(string name, string? parentId = null)
    {
        var body = new { name, parent_id = parentId };
        var response = await _httpClient.PostAsJsonAsync($"{BaseUrl}/api/v2/files/folders", body);
        response.EnsureSuccessStatusCode();
        return await response.Content.ReadFromJsonAsync<FileItem>()
               ?? throw new Exception("创建文件夹失败");
    }

    #endregion
}
```

### 4.3 创建单例服务

为了让整个应用共享同一个 API 客户端，我们需要用**依赖注入（DI）**：

**`App.xaml.cs`** 修改如下：

```csharp
using Microsoft.Extensions.DependencyInjection;
using StoraDesktop.Services;
using StoraDesktop.Views;
using StoraDesktop.ViewModels;

namespace StoraDesktop;

public partial class App : Application
{
    // 全局服务容器
    public static IServiceProvider Services { get; private set; } = null!;

    public App()
    {
        this.InitializeComponent();

        // 注册所有服务
        var services = new ServiceCollection();
        services.AddSingleton<StoraApiClient>();
        services.AddTransient<LoginViewModel>();
        services.AddTransient<MainViewModel>();
        services.AddTransient<LoginPage>();
        services.AddTransient<MainPage>();
        // ... 以后每加一个页面/ViewModel，都在这里注册

        Services = services.BuildServiceProvider();
    }

    protected override void OnLaunched(Microsoft.UI.Xaml.LaunchActivatedEventArgs args)
    {
        m_window = new MainWindow();
        m_window.Activate();
    }

    private Window? m_window;
}
```

---

## 5. 登录界面

### 5.1 ViewModel（登录逻辑）

**`ViewModels/LoginViewModel.cs`**：

```csharp
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using StoraDesktop.Services;

namespace StoraDesktop.ViewModels;

public partial class LoginViewModel : ObservableObject
{
    private readonly StoraApiClient _apiClient;

    [ObservableProperty]
    private string _username = string.Empty;

    [ObservableProperty]
    private string _password = string.Empty;

    [ObservableProperty]
    private string _serverUrl = "http://localhost:9421";

    [ObservableProperty]
    private string _errorMessage = string.Empty;

    [ObservableProperty]
    private bool _isLoading;

    public LoginViewModel(StoraApiClient apiClient)
    {
        _apiClient = apiClient;
    }

    [RelayCommand]
    private async Task LoginAsync()
    {
        if (string.IsNullOrWhiteSpace(Username) || string.IsNullOrWhiteSpace(Password))
        {
            ErrorMessage = "请输入用户名和密码";
            return;
        }

        try
        {
            IsLoading = true;
            ErrorMessage = string.Empty;

            // 设置服务器地址
            _apiClient.BaseUrl = ServerUrl.TrimEnd('/');

            // 调用登录 API
            var result = await _apiClient.LoginAsync(Username, Password);

            // 👉 登录成功，保存令牌并跳转到主页面
            // （具体导航逻辑稍后实现）
        }
        catch (Exception ex)
        {
            ErrorMessage = $"登录失败：{ex.Message}";
        }
        finally
        {
            IsLoading = false;
        }
    }
}
```

### 5.2 XAML 页面

**`Views/LoginPage.xaml`**：

```xml
<?xml version="1.0" encoding="utf-8"?>
<Page x:Class="StoraDesktop.Views.LoginPage"
      xmlns="http://schemas.microsoft.com/winfx/2006/xaml/presentation"
      xmlns:x="http://schemas.microsoft.com/winfx/2006/xaml">

    <Grid Background="{ThemeResource ApplicationPageBackgroundThemeBrush}">
        <!-- 居中登录卡片 -->
        <StackPanel HorizontalAlignment="Center"
                    VerticalAlignment="Center"
                    Spacing="16"
                    Width="380"
                    Padding="32">

            <!-- Logo / 标题 -->
            <TextBlock Text="Stora"
                       FontSize="36"
                       FontWeight="Bold"
                       TextAlignment="Center"
                       Foreground="{ThemeResource SystemAccentColor}" />

            <TextBlock Text="Windows 桌面客户端"
                       FontSize="14"
                       TextAlignment="Center"
                       Margin="0,0,0,24"
                       Opacity="0.6" />

            <!-- 服务器地址 -->
            <TextBox x:Name="ServerUrlBox"
                     Header="服务器地址"
                     PlaceholderText="http://localhost:9421"
                     Text="{x:Bind ViewModel.ServerUrl, Mode=TwoWay, UpdateSourceTrigger=PropertyChanged}" />

            <!-- 用户名 -->
            <TextBox Header="用户名"
                     PlaceholderText="请输入用户名"
                     Text="{x:Bind ViewModel.Username, Mode=TwoWay, UpdateSourceTrigger=PropertyChanged}" />

            <!-- 密码 -->
            <PasswordBox Header="密码"
                         PlaceholderText="请输入密码"
                         Password="{x:Bind ViewModel.Password, Mode=TwoWay, UpdateSourceTrigger=PropertyChanged}" />

            <!-- 错误提示 -->
            <TextBlock Text="{x:Bind ViewModel.ErrorMessage, Mode=OneWay}"
                       Foreground="Red"
                       Visibility="{x:Bind ViewModel.ErrorMessage, Mode=OneWay, Converter={StaticResource StringToVisibilityConverter}}" />

            <!-- 登录按钮 -->
            <Button Content="登 录"
                    Command="{x:Bind ViewModel.LoginCommand}"
                    HorizontalAlignment="Stretch"
                    Height="40"
                    Style="{StaticResource AccentButtonStyle}" />

            <!-- 加载指示器 -->
            <ProgressRing IsActive="{x:Bind ViewModel.IsLoading, Mode=OneWay}"
                          HorizontalAlignment="Center"
                          Visibility="{x:Bind ViewModel.IsLoading, Mode=OneWay}" />
        </StackPanel>
    </Grid>
</Page>
```

**`Views/LoginPage.xaml.cs`**：

```csharp
using StoraDesktop.ViewModels;

namespace StoraDesktop.Views;

public sealed partial class LoginPage : Page
{
    public LoginViewModel ViewModel { get; }

    public LoginPage(LoginViewModel viewModel)
    {
        this.InitializeComponent();
        ViewModel = viewModel;
    }
}
```

---

## 6. 主界面 — 文件浏览器

### 6.1 ViewModel

**`ViewModels/MainViewModel.cs`**：

```csharp
using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using StoraDesktop.Models;
using StoraDesktop.Services;

namespace StoraDesktop.ViewModels;

public partial class MainViewModel : ObservableObject
{
    private readonly StoraApiClient _apiClient;

    [ObservableProperty]
    private string _currentPath = "/";

    [ObservableProperty]
    private string? _currentFolderId;

    [ObservableProperty]
    private bool _isLoading;

    [ObservableProperty]
    private string _statusText = "就绪";

    [ObservableProperty]
    private UserInfo? _currentUser;

    public ObservableCollection<FileItem> Files { get; } = new();

    public MainViewModel(StoraApiClient apiClient)
    {
        _apiClient = apiClient;
    }

    /// <summary>
    /// 初始化（加载用户信息和文件列表）
    /// </summary>
    [RelayCommand]
    private async Task InitializeAsync()
    {
        try
        {
            IsLoading = true;
            CurrentUser = await _apiClient.GetCurrentUserAsync();
            await LoadFilesAsync();
        }
        finally
        {
            IsLoading = false;
        }
    }

    /// <summary>
    /// 加载文件列表
    /// </summary>
    [RelayCommand]
    private async Task LoadFilesAsync()
    {
        try
        {
            IsLoading = true;
            StatusText = "正在加载...";

            var result = await _apiClient.GetFilesAsync(CurrentFolderId);
            Files.Clear();
            foreach (var file in result.Data)
            {
                Files.Add(file);
            }
            StatusText = $"共 {result.Total} 个项目";
        }
        finally
        {
            IsLoading = false;
        }
    }

    /// <summary>
    /// 双击进入文件夹
    /// </summary>
    [RelayCommand]
    private async Task OpenFolderAsync(FileItem folder)
    {
        if (!folder.IsFolder) return;
        CurrentFolderId = folder.Id;
        CurrentPath += $"/{folder.Name}";
        await LoadFilesAsync();
    }

    /// <summary>
    /// 返回上一级
    /// </summary>
    [RelayCommand]
    private async Task GoBackAsync()
    {
        // 简化处理：回到根目录
        CurrentFolderId = null;
        CurrentPath = "/";
        await LoadFilesAsync();
    }

    /// <summary>
    /// 下载文件
    /// </summary>
    [RelayCommand]
    private async Task DownloadFileAsync(FileItem file)
    {
        // 弹出保存对话框
        var savePicker = new Windows.Storage.Pickers.FileSavePicker();
        savePicker.SuggestedFileName = file.Name;

        var window = new Window(); // 获取当前窗口
        // WinUI 3 中需要初始化 picker
        WinRT.Interop.InitializeWithWindow.Initialize(
            savePicker, WinRT.Interop.WindowNative.GetWindowHandle(window));

        var saveFile = await savePicker.PickSaveFileAsync();
        if (saveFile == null) return;

        try
        {
            StatusText = $"正在下载 {file.Name}...";
            var stream = await _apiClient.DownloadFileAsync(file.Id);
            using var fileStream = await saveFile.OpenStreamForWriteAsync();
            await stream.CopyToAsync(fileStream);
            StatusText = $"{file.Name} 下载完成";
        }
        catch (Exception ex)
        {
            StatusText = $"下载失败：{ex.Message}";
        }
    }

    /// <summary>
    /// 登出
    /// </summary>
    [RelayCommand]
    private async Task LogoutAsync()
    {
        await _apiClient.LogoutAsync();
        // 👉 跳转到登录页面
    }
}
```

### 6.2 文件列表 XAML

**`Views/MainPage.xaml`**（文件浏览器部分）：

```xml
<?xml version="1.0" encoding="utf-8"?>
<Page x:Class="StoraDesktop.Views.MainPage"
      xmlns="http://schemas.microsoft.com/winfx/2006/xaml/presentation"
      xmlns:x="http://schemas.microsoft.com/winfx/2006/xaml">

    <Grid>
        <Grid.RowDefinitions>
            <RowDefinition Height="48" />
            <RowDefinition Height="*" />
            <RowDefinition Height="32" />
        </Grid.RowDefinitions>

        <!-- 顶部工具栏 -->
        <Grid Grid.Row="0" Padding="12,8" Background="{ThemeResource SystemControlBackgroundChromeMediumBrush}">
            <Grid.ColumnDefinitions>
                <ColumnDefinition Width="Auto" />
                <ColumnDefinition Width="*" />
                <ColumnDefinition Width="Auto" />
            </Grid.ColumnDefinitions>

            <!-- 返回按钮 -->
            <Button Grid.Column="0"
                    Content="⬅ 返回"
                    Command="{x:Bind ViewModel.GoBackCommand}"
                    Margin="0,0,12,0" />

            <!-- 当前路径 -->
            <TextBlock Grid.Column="1"
                       Text="{x:Bind ViewModel.CurrentPath, Mode=OneWay}"
                       VerticalAlignment="Center"
                       FontSize="16" />

            <!-- 用户信息 -->
            <StackPanel Grid.Column="2" Orientation="Horizontal" Spacing="8">
                <TextBlock Text="{x:Bind ViewModel.CurrentUser.Username, Mode=OneWay}"
                           VerticalAlignment="Center" />
                <Button Content="登出"
                        Command="{x:Bind ViewModel.LogoutCommand}" />
            </StackPanel>
        </Grid>

        <!-- 文件列表 (采用 ListView) -->
        <ListView Grid.Row="1"
                  ItemsSource="{x:Bind ViewModel.Files, Mode=OneWay}"
                  SelectionMode="Single"
                  DoubleTapped="OnFileDoubleTapped">
            <ListView.ItemTemplate>
                <DataTemplate x:DataType="models:FileItem">
                    <Grid Padding="8,4" ColumnSpacing="12">
                        <Grid.ColumnDefinitions>
                            <ColumnDefinition Width="32" />
                            <ColumnDefinition Width="*" />
                            <ColumnDefinition Width="120" />
                            <ColumnDefinition Width="160" />
                        </Grid.ColumnDefinitions>

                        <!-- 图标：文件夹 / 文件 -->
                        <TextBlock Grid.Column="0"
                                   Text="{x:Bind IsFolder, Converter={StaticResource FolderIconConverter}}"
                                   FontSize="20" />

                        <!-- 文件名 -->
                        <TextBlock Grid.Column="1" Text="{x:Bind Name}" />

                        <!-- 文件大小 -->
                        <TextBlock Grid.Column="2" Text="{x:Bind SizeDisplay}" />

                        <!-- 修改时间 -->
                        <TextBlock Grid.Column="3"
                                   Text="{x:Bind UpdatedAt, Converter={StaticResource DateTimeConverter}}" />
                    </Grid>
                </DataTemplate>
            </ListView.ItemTemplate>
        </ListView>

        <!-- 底部状态栏 -->
        <TextBlock Grid.Row="2"
                   Text="{x:Bind ViewModel.StatusText, Mode=OneWay}"
                   Padding="12,4"
                   FontSize="12"
                   Opacity="0.6"
                   Background="{ThemeResource SystemControlBackgroundChromeMediumBrush}" />
    </Grid>
</Page>
```

**`Views/MainPage.xaml.cs`**：

```csharp
using StoraDesktop.Models;
using StoraDesktop.ViewModels;

namespace StoraDesktop.Views;

public sealed partial class MainPage : Page
{
    public MainViewModel ViewModel { get; }

    public MainPage(MainViewModel viewModel)
    {
        this.InitializeComponent();
        ViewModel = viewModel;
        this.Loaded += async (s, e) => await ViewModel.InitializeCommand.ExecuteAsync(null);
    }

    private void OnFileDoubleTapped(object sender, Microsoft.UI.Xaml.Input.DoubleTappedRoutedEventArgs e)
    {
        if ((e.OriginalSource as FrameworkElement)?.DataContext is FileItem file && file.IsFolder)
        {
            ViewModel.OpenFolderCommand.Execute(file);
        }
    }
}
```

---

## 7. 上传与下载

### 7.1 带进度条的上传

WinUI 3 支持真正的异步文件操作。下面是上传功能的增强实现：

在 **MainViewModel.cs** 中添加：

```csharp
/// <summary>
/// 上传文件（显示进度）
/// </summary>
[RelayCommand]
private async Task UploadFileAsync()
{
    var picker = new Windows.Storage.Pickers.FileOpenPicker();
    picker.FileTypeFilter.Add("*"); // 允许所有类型

    // WinUI 3 需要初始化窗口句柄
    var hwnd = WinRT.Interop.WindowNative.GetWindowHandle(App.MainWindow);
    WinRT.Interop.InitializeWithWindow.Initialize(picker, hwnd);

    var files = await picker.PickMultipleFilesAsync();
    if (files == null || files.Count == 0) return;

    foreach (var file in files)
    {
        try
        {
            StatusText = $"正在上传 {file.Name}...";
            using var stream = await file.OpenStreamForReadAsync();
            await _apiClient.UploadFileAsync(stream, file.Name, CurrentFolderId);
            StatusText = $"{file.Name} 上传完成";
        }
        catch (Exception ex)
        {
            StatusText = $"上传 {file.Name} 失败：{ex.Message}";
        }
    }

    // 刷新文件列表
    await LoadFilesAsync();
}
```

### 7.2 拖拽上传

在主页面 XAML 中添加拖拽支持：

```xml
<!-- ListView 上启用拖放 -->
<ListView AllowDrop="True"
          Drop="OnDrop"
          DragOver="OnDragOver"
          ... />
```

在 `MainPage.xaml.cs` 中：

```csharp
private void OnDragOver(object sender, DragEventArgs e)
{
    e.AcceptedOperation = Windows.ApplicationModel.DataTransfer.DataPackageOperation.Copy;
    e.DragUIOverride.Caption = "上传到当前目录";
}

private async void OnDrop(object sender, DragEventArgs e)
{
    if (e.DataView.Contains(Windows.ApplicationModel.DataTransfer.StandardDataFormats.StorageItems))
    {
        var items = await e.DataView.GetStorageItemsAsync();
        foreach (var item in items.OfType<StorageFile>())
        {
            using var stream = await item.OpenStreamForReadAsync();
            await ViewModel.ApiClient.UploadFileAsync(stream, item.Name, ViewModel.CurrentFolderId);
        }
        await ViewModel.LoadFilesCommand.ExecuteAsync(null);
    }
}
```

---

## 8. 系统托盘与后台运行

这是桌面端相比 Web 端的核心优势之一。

### 8.1 添加系统托盘图标

**在 `App.xaml.cs` 中添加**：

```csharp
using Microsoft.UI.Xaml;
using Windows.ApplicationModel.Core;
using Windows.UI.Core;

public partial class App : Application
{
    private Window? m_window;

    // 系统托盘图标
    private HIcon? _trayIcon;
    private const int WM_TRAY_CALLBACK = 0x8000;

    protected override void OnLaunched(Microsoft.UI.Xaml.LaunchActivatedEventArgs args)
    {
        m_window = new MainWindow();
        m_window.Closed += OnWindowClosed;
        m_window.Activate();

        // 创建系统托盘
        CreateTrayIcon();
    }

    private void CreateTrayIcon()
    {
        // 💡 专业提示：WinUI 3 系统托盘使用 Windows API
        // 这是一个简化版本，实际需要使用 P/Invoke 调用 Shell_NotifyIcon
        //
        // 推荐使用社区库：https://github.com/mrousavy/H.NotifyIcon
        // 安装：dotnet add package H.NotifyIcon
        //
        // 简化用法示例：
        // var notifyIcon = new NotifyIcon();
        // notifyIcon.Icon = new Icon("app.ico");
        // notifyIcon.Text = "Stora Desktop";
        // notifyIcon.Visible = true;
        // notifyIcon.Click += (s, e) => ShowWindow();
    }

    private void OnWindowClosed(object sender, WindowEventArgs args)
    {
        // 用户点击关闭时，最小化到托盘而不是退出
        args.Handled = true;
        m_window.Hide();
    }

    /// <summary>
    /// 显示窗口
    /// </summary>
    public void ShowWindow()
    {
        m_window?.DispatcherQueue.TryEnqueue(() =>
        {
            m_window?.Show();
        });
    }
}
```

### 8.2 后台运行与自动启动

**本地服务器模式** — 后台启动 Go 进程：

```csharp
using System.Diagnostics;

/// <summary>
/// 启动本地 Stora 服务器
/// </summary>
public class LocalServerService
{
    private Process? _serverProcess;

    public bool IsRunning => _serverProcess?.HasExited == false;

    public async Task StartServerAsync(string serverExePath)
    {
        if (IsRunning) return;

        _serverProcess = new Process
        {
            StartInfo = new ProcessStartInfo
            {
                FileName = serverExePath,
                Arguments = "serve",       // Stora CLI 的 serve 命令
                UseShellExecute = false,
                CreateNoWindow = true,     // 不显示控制台窗口
                RedirectStandardOutput = true,
                RedirectStandardError = true,
            }
        };

        _serverProcess.Start();

        // 等待服务器就绪
        await WaitForServerReadyAsync();
    }

    public void StopServer()
    {
        if (_serverProcess?.HasExited == false)
        {
            _serverProcess.Kill();
            _serverProcess.WaitForExit(5000);
            _serverProcess.Dispose();
            _serverProcess = null;
        }
    }

    private async Task WaitForServerReadyAsync()
    {
        using var client = new HttpClient();
        var maxRetries = 30;
        for (int i = 0; i < maxRetries; i++)
        {
            try
            {
                var response = await client.GetAsync("http://localhost:9421/api/health");
                if (response.IsSuccessStatusCode) return;
            }
            catch
            {
                // 服务器还没启动，继续等待
            }
            await Task.Delay(1000);
        }
        throw new TimeoutException("服务器启动超时");
    }
}
```

---

## 9. 本地服务器模式

### 9.1 设置页面

用户需要在设置中选择"本地模式"还是"远程模式"。

**`Views/SettingsPage.xaml`**：

```xml
<Page x:Class="StoraDesktop.Views.SettingsPage" ...>
    <StackPanel Spacing="16" Padding="24">
        <TextBlock Text="设置" FontSize="28" FontWeight="Bold" />

        <!-- 连接模式 -->
        <ComboBox Header="连接模式" SelectedIndex="{x:Bind ViewModel.ConnectionMode, Mode=TwoWay}">
            <x:String>本地模式（启动内置服务器）</x:String>
            <x:String>远程模式（连接到已有服务器）</x:String>
        </ComboBox>

        <!-- 远程服务器地址 -->
        <TextBox Header="远程服务器地址"
                 Text="{x:Bind ViewModel.RemoteServerUrl, Mode=TwoWay}"
                 IsEnabled="{x:Bind ViewModel.IsRemoteMode, Mode=OneWay}" />

        <!-- 本地服务器路径 -->
        <TextBox Header="本地服务器程序路径"
                 Text="{x:Bind ViewModel.LocalServerPath, Mode=TwoWay}"
                 IsEnabled="{x:Bind ViewModel.IsLocalMode, Mode=OneWay}" />

        <!-- 启动/停止本地服务器 -->
        <StackPanel Orientation="Horizontal" Spacing="12">
            <Button Content="启动本地服务器"
                    Command="{x:Bind ViewModel.StartLocalServerCommand}" />
            <Button Content="停止本地服务器"
                    Command="{x:Bind ViewModel.StopLocalServerCommand}" />
        </StackPanel>

        <!-- 开机自启 -->
        <ToggleSwitch Header="开机自动启动" OnContent="开启" OffContent="关闭" />
    </StackPanel>
</Page>
```

### 9.2 快速切换模式

在**主界面工具栏**添加一个快速切换按钮，让用户随时切换本地/远程服务器。

---

## 10. 打包发布

### 10.1 生成安装包

1. 右键项目 → **发布** → **创建 App 包**
2. 选择 **旁加载 (Sideloading)** 或 **Windows Store**
3. 配置签名证书：
   - 如果是个人使用，选择**测试证书**
   - 如果要分发，需要购买代码签名证书
4. 选择架构：`x64`
5. 点击**创建**

发布后在 `AppPackages/` 目录下会生成 `.msix` 或 `.msixbundle` 安装包。

### 10.2 免商店分发

用户可以双击 `.msix` 文件安装：

1. 双击安装包
2. 点击**安装**
3. 开始在开始菜单中找到 Stora

> 💡 **提示**：如果是第一次安装旁加载应用，需要在"设置 → 更新和安全 → 开发者选项"中启用**旁加载应用**

### 10.3 自动更新

WinUI 3 支持自动更新（需配合 Windows 应用安装程序）：

1. 在项目中启用 **Generate App Installer**
2. 将生成的 `.appinstaller` 文件部署到网页服务器
3. 每次更新只需要上传新的安装包，用户下次打开会自动更新

---

## 🎯 开发路线图（按优先级）

| 阶段 | 功能 | 预计工作量 |
|------|------|-----------|
| **Phase 1** | 项目搭建 + 登录 + 文件浏览 | 1-2 天 |
| **Phase 2** | 上传/下载 + 文件夹管理 | 1-2 天 |
| **Phase 3** | 系统托盘 + 后台运行 + 本地服务器模式 | 1 天 |
| **Phase 4** | 文件分享 + 回收站 | 1 天 |
| **Phase 5** | 搜索 + 收藏 + 标签 | 1 天 |
| **Phase 6** | 保险箱（Vault）+ 加密 | 2 天 |
| **Phase 7** | 设置页面 + 主题切换 | 1 天 |
| **Phase 8** | 打包发布 + 自动更新 | 1 天 |

---

## ❓ 常见问题

### Q：需要学多久 C# 才能写这个？

**不用提前学**！这份教程里的代码可以直接复制使用。建议边写边学：
- 遇到不懂的语法，右键 → **转到定义** 看说明
- 用 **IntelliSense**（智能提示），打字时自动补全
- 上网搜："C# WinUI 3 怎么实现 XXX"

### Q：没有服务器怎么测试？

两种方式：
1. **本地模式**：先编译运行你的 Go 后端（`go run ./cmd/server`），客户端连接 `http://localhost:9421`
2. **远程模式**：如果你有部署好的 Stora 服务，填上地址即可

### Q：UI 怎么做漂亮？

WinUI 3 自带**现代 Fluent Design** 风格，天然就好看。想更进一步：
- 使用 **WinUI 3 Gallery** 应用参考微软的设计
- 在 [Figma](https://figma.com) 上先画界面设计稿
- 使用 **AcrylicBrush**（亚克力效果）和 **MicaBrush**（云母效果）让背景半透明
- 加入**深色模式**支持

### Q：遇到问题怎么办？

1. **搜索引擎**：用中文搜"WinUI 3 xxx 教程"
2. **微软文档**：https://learn.microsoft.com/zh-cn/windows/apps/winui/
3. **Stack Overflow**：搜 `[winui3]` 标签
4. **GitHub 示例**：https://github.com/microsoft/WinUI-Gallery — 官方示例应用

---

> 📌 **下一步**：上面教程讲完了所有核心概念。你准备好了之后，告诉我，我们开始 **第 2 步：创建项目**，我会手把手带你操作！
